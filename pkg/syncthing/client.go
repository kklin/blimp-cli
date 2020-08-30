package syncthing

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	rice "github.com/GeertJohan/go.rice"
	log "github.com/sirupsen/logrus"

	"github.com/kelda/blimp/pkg/cfgdir"
	"github.com/kelda/blimp/pkg/errors"
	"github.com/kelda/blimp/pkg/hash"
	"github.com/kelda/blimp/pkg/proto/node"
	"github.com/kelda/blimp/pkg/strs"
	"github.com/kelda/blimp/pkg/tunnel"
)

const stignoreHeader = `# Generated by Blimp. DO NOT EDIT.
# This file is used by Blimp to control what files are synced.`

type Client struct {
	mounts []Mount
}

func (c Client) GetIDPathMap() map[string]string {
	idPathMap := map[string]string{}
	for _, m := range c.mounts {
		idPathMap[m.ID()] = m.Path
	}
	return idPathMap
}

// BindVolume represents a bind volume used by a single service, along with any
// subdirectories that are masked off by native volumes mounted into this
// service.
type BindVolume struct {
	LocalPath string
	Masks     []string
}

type Mount struct {
	Path string
	// Include should only be set if Ignore is nil and SyncAll is false.
	Include []string
	// Ignore overrides SyncAll for specific files or directories. Ignore should
	// only be set if Include is nil and SyncAll is true.
	Ignore  []string
	SyncAll bool
}

// GetStignore returns the stignore file needed to include only the paths in
// `Mount.Include`.
// In order to sync subdirectories, we have to add explicit rules allowing each
// parent directory, and ignoring all other files in the parent directory. For example,
// to sync the directory /foo/bar/, we have to use a stignore of the form:
// ```
// !/foo/bar # Include the target directory.
// /foo/	 # Exclude all other files in /foo, but not /foo itself.
// !/foo     # Include /foo.
// ```
// If we didn't have the latter two rules, /foo/bar wouldn't get synced, since
// the directory /foo would never get created.
// See this issue for more information: https://github.com/syncthing/syncthing/issues/2091.
func (m Mount) GetStignore() (stignore string, needed bool) {
	var allRules []string
	for _, include := range m.Include {
		allRules = append(allRules, rulesToIncludePath(include)...)
	}
	for _, ignore := range m.Ignore {
		allRules = append(allRules, fmt.Sprintf("/%s", ignore))
	}
	allRules = strs.Unique(allRules)

	// Sort the rules so that more specific rules are matched first.
	// This is necessary to properly merge rules when there are multiple
	// Includes, since any rules to explicitly include files/directories must
	// come before the exclusion rule for its parent.
	sort.Slice(allRules, func(i, j int) bool {
		left := allRules[i]
		right := allRules[j]

		// If the rule is for a path deeper in the filesystem tree, it's more
		// specific.
		depth := func(rule string) int {
			return strings.Count(filepath.Clean(rule), "/")
		}
		if depth(left) > depth(right) {
			return true
		}
		if depth(right) > depth(left) {
			return false
		}

		// A directory with a trailing / is more specific, since it only
		// matches the directory's contents, and doesn't match the directory's
		// name.
		onlyDirContents := func(rule string) bool {
			return strings.HasSuffix(rule, "/")
		}
		if onlyDirContents(left) && !onlyDirContents(right) {
			return true
		}
		if !onlyDirContents(left) && onlyDirContents(right) {
			return false
		}

		// The rules are siblings, and their order doesn't matter.
		return left < right
	})

	if !m.SyncAll {
		allRules = append(allRules, "\n# Ignore all other files.\n**")
	}

	if len(allRules) == 0 {
		return "", false
	}

	return fmt.Sprintf(`%s

%s
`, stignoreHeader, strings.Join(allRules, "\n")), true
}

func (m Mount) ID() string {
	return hash.DNSCompliant(m.Path)
}

// Includes returns whether the mount already includes the given child.
func (m Mount) Includes(child string) bool {
	for _, include := range m.Include {
		_, ok := getSubpath(include, child)
		if ok {
			return true
		}
	}
	return false
}

func NewClient(volumes []BindVolume) Client {
	var allMounts []Mount
	// Collect all the mounts, regardless of whether they're nested.
	for _, volume := range volumes {
		// For directories, we just mount the entire directory. For other
		// files, we mount the parent directory, and use .stignore to only sync
		// the desired files.
		if isDir(volume.LocalPath) {
			allMounts = append(allMounts, Mount{
				Path:    volume.LocalPath,
				SyncAll: true,
				Ignore:  collapseIgnores(volume.Masks),
			})
		} else {
			if len(volume.Masks) > 0 {
				log.WithField("volume", volume).Warn("Volume has masked subdirectories, but is not a directory. Ignoring.")
			}
			allMounts = append(allMounts, Mount{
				Path:    filepath.Dir(volume.LocalPath),
				Include: []string{filepath.Base(volume.LocalPath)},
			})
		}
	}

	// Collapse nested mounts to avoid confusing Syncthing.
	// TODO: In some cases, we will lose "Ignore" informtaion that could be
	// kept, leading us to sync things that may technically not need to be
	// synced. However, avoiding all such cases would require either a complete
	// restructuring of this logic, or a ton of spaghetti code. We should fix
	// this eventually.

	// We first sort the mounts from shallowest to deepest. This way, we know
	// that the first mount that matches a path is the most efficient match.
	sort.Slice(allMounts, func(i, j int) bool {
		depth := func(path string) int {
			return strings.Count(path, string(filepath.Separator))
		}
		return depth(allMounts[i].Path) < depth(allMounts[j].Path)
	})

	// Starting from the highest-level directories, try to greedily combine
	// child mounts.
	var collapsedMounts []Mount
	// skipIndices tracks mounts that have been collapsed already.
	skipIndices := map[int]struct{}{}
	for pi, parent := range allMounts {
		if _, ok := skipIndices[pi]; ok {
			continue
		}

		for mi := pi + 1; mi < len(allMounts); mi++ {
			if _, ok := skipIndices[mi]; ok {
				continue
			}

			mount := allMounts[mi]
			relPath, ok := getSubpath(parent.Path, mount.Path)
			if !ok {
				// The mount isn't nested within the parent.
				continue
			}

			switch {
			case parent.SyncAll:
				// If the parent already syncs this mount, then any includes for
				// files don't matter since they'll be automatically picked up.
				// However, we must filter out any potentially problematic
				// Ignores.
				var newIgnore []string
				for _, parentIgnore := range parent.Ignore {
					// Make sure that this ignore does not mask the child.
					if _, ok := getSubpath(parentIgnore, relPath); ok {
						// Don't add the ignore to newIgnore. Could be overkill.
						continue
					}

					// If the ignore falls under the child path, only add if
					// it's also ignored by the child.
					relIgnore, ok := getSubpath(relPath, parentIgnore)
					if !ok {
						// The ignore is unrelated to the child. We can add it.
						newIgnore = append(newIgnore, parentIgnore)
					} else if mount.SyncAll {
						// Look for a matching ignore in the child.
						for _, mountIgnore := range mount.Ignore {
							if mountIgnore == relIgnore {
								// We found a match!
								newIgnore = append(newIgnore, parentIgnore)
								break
							}
						}

						// No match found
					}
				}
				parent.Ignore = newIgnore

			case parent.Includes(relPath):
				// This mount will already be included. Ignores may be
				// discarded. Too bad.
				if len(mount.Ignore) > 0 {
					// We don't support ignores nested under an include,
					// unfortunately.
					log.WithField("Ignore", mount.Ignore).Debug("Dropping ignores from include.")
				}

			// If we're syncing an entire subdirectory, add the relative
			// path to the subdirectory to the include list. Child ignores may
			// be discarded, so be it.
			case mount.SyncAll:
				// If it's the same path, then just modify the parent to sync
				// all the files.
				if relPath == "." {
					parent.SyncAll = true
					parent.Include = nil
				} else {
					// Otherwise, add the relative path to the subdirectory to the
					// include list.
					parent.Include = append(parent.Include, relPath)
				}
				if len(mount.Ignore) > 0 {
					// It is too complicated to calculate which ignores can be
					// kept.
					log.WithField("Ignore", mount.Ignore).Debug("Dropping child ignores from updated parent mount.")
				}

			// If we're syncing an individual file, add the individual file to
			// the include list.
			default:
				for _, include := range mount.Include {
					parent.Include = append(parent.Include, filepath.Join(relPath, include))
				}
			}
			skipIndices[mi] = struct{}{}
		}

		collapsedMounts = append(collapsedMounts, parent)
	}

	return Client{
		mounts: collapsedMounts,
	}
}

func (c Client) Run(ctx context.Context, ncc node.ControllerClient,
	token string, tunnelManager tunnel.Manager) ([]byte, error) {

	tunnelsErr := c.startTunnels(tunnelManager)

	idPathMap := c.GetIDPathMap()
	if err := c.WriteConfig(idPathMap); err != nil {
		return nil, errors.WithContext("write config", err)
	}

	finishedInitialSync := make(chan struct{}, 1)
	go runSyncCompletionServer(ctx, ncc, token, finishedInitialSync)

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, stbinPath(), "-verbose", "-home", cfgdir.Expand(""),
		"-logfile", cfgdir.Expand("syncthing.log"))
	cmd.Stderr = &out
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return nil, errors.WithContext("start syncthing", err)
	}

	initialSyncCtx, cancelInitialSync := context.WithCancel(ctx)
	waitErrChan := make(chan error)
	go func() {
		waitErrChan <- cmd.Wait()
		cancelInitialSync()
	}()

	initialSyncErr := make(chan error)
	go func() {
		initialSyncErr <- c.performInitialSync(initialSyncCtx, fmt.Sprintf("127.0.0.1:%d", TunneledAPIPort), idPathMap)
	}()

	select {
	// If Syncthing crashes, abort the sync, and return immediately.
	case err := <-waitErrChan:
		cancelInitialSync()
		return out.Bytes(), errors.WithContext("syncthing crashed", err)
	case err := <-initialSyncErr:
		if err != nil {
			return nil, errors.WithContext("initial sync failed", err)
		}
	case err := <-tunnelsErr:
		if err != nil {
			return nil, errors.WithContext("tunnel crashed", err)
		}
	}

	finishedInitialSync <- struct{}{}

	select {
	case err := <-tunnelsErr:
		if err != nil {
			return nil, errors.WithContext("tunnel crashed", err)
		}
	case waitErr := <-waitErrChan:
		return out.Bytes(), waitErr
	}
	panic("unreached")
}

func (c Client) startTunnels(tm tunnel.Manager) chan error {
	tunnels := []struct {
		localPort, remotePort uint32
	}{
		{Port, Port},
		{TunneledAPIPort, APIPort},
	}

	errChan := make(chan error, 1)
	for _, tunnel := range tunnels {
		tunnel := tunnel
		go func() {
			select {
			// TODO: This will block until it exits right?
			case errChan <- tm.Run("127.0.0.1", tunnel.localPort, "syncthing", tunnel.remotePort, nil):
			default:
			}
		}()
	}
	return errChan
}

func (c Client) performInitialSync(ctx context.Context, remoteAPIAddr string, idPathMap map[string]string) error {
	localAPI := APIClient{fmt.Sprintf("127.0.0.1:%d", APIPort)}
	remoteAPI := APIClient{remoteAPIAddr}

	var folders []string
	for folder := range idPathMap {
		folders = append(folders, folder)
	}

	// Wait for the Syncthing daemons to boot. The connections may fail at
	// first since the tunnels are started asynchronously.
	waitCtx, _ := context.WithTimeout(ctx, 5*time.Minute)
	if err := waitUntilConnected(waitCtx, localAPI, remoteAPI); err != nil {
		return errors.WithContext("wait for devices to connect", err)
	}

	if err := waitUntilScanned(ctx, localAPI, folders); err != nil {
		return errors.WithContext("wait for initial scan", err)
	}

	if err := waitUntilSynced(ctx, localAPI, folders); err != nil {
		return errors.WithContext("wait for initial sync", err)
	}

	if err := setLocalFolderType(ctx, localAPI, "sendreceive", idPathMap); err != nil {
		return errors.WithContext("switch to sendreceive", err)
	}

	return nil
}

func (c Client) WriteConfig(idPathMap map[string]string) error {
	err := MakeMarkers(idPathMap)
	if err != nil {
		return errors.WithContext("make markers", err)
	}

	for _, m := range c.mounts {
		// Remove any stale stignores. This avoids an issue where a previous
		// run mounts a file in a directory, which creates an stignore, and the
		// user modifies their volumes to mount the entire directory.
		// In that case, we should run with no stignore at all.
		if contents, err := ioutil.ReadFile(".stignore"); err == nil {
			if strings.Contains(string(contents), stignoreHeader) {
				if err := os.Remove(".stignore"); err != nil {
					return errors.WithContext("remove stignore", err)
				}
			}
		}

		stignore, ok := m.GetStignore()
		if !ok {
			continue
		}

		path := filepath.Join(m.Path, ".stignore")
		err := ioutil.WriteFile(path, []byte(stignore), 0644)
		if err != nil {
			return errors.WithContext("write stignore", err)
		}

		go ensureFileExists(path, stignore)
	}

	box := rice.MustFindBox("stbin")
	stbinBytes, err := box.Bytes("")
	if err != nil {
		// This really really can't happen as stbin is supposed to be
		// literally embedded in this binary.  A panic is actually
		// appropriate.
		panic(err)
	}

	err = ioutil.WriteFile(stbinPath(), stbinBytes, 0755)
	if err != nil {
		return errors.WithContext("write stbin error", err)
	}

	fileMap := map[string]string{
		"config.xml": makeConfig(false, idPathMap, "sendonly"),
		"cert.pem":   cert,
		"key.pem":    key,
	}
	for path, data := range fileMap {
		err := ioutil.WriteFile(cfgdir.Expand(path), []byte(data), 0644)
		if err != nil {
			return errors.WithContext("write config", err)
		}
	}

	return nil
}

func stbinPath() string {
	if runtime.GOOS == "windows" {
		return cfgdir.Expand("stbin.exe")
	}

	return cfgdir.Expand("stbin")
}

func ensureFileExists(path, contents string) {
	for {
		time.Sleep(30 * time.Second)
		if f, err := os.Open(path); err == nil {
			// File exists.
			f.Close()
			continue
		}
		err := ioutil.WriteFile(path, []byte(contents), 0644)
		if err != nil {
			log.WithField("path", path).WithError(err).Warn("Failed to write file")
		}
	}
}

var isDir = func(path string) bool {
	fi, err := os.Stat(path)

	// Create the path as a directory if it doesn't exist.
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			log.
				WithError(err).
				WithField("path", path).
				Fatal("Tried to create directory for non-existent bind volume, but failed")
		}
		return true
	}

	return err == nil && fi.IsDir()
}

func getSubpath(parent, child string) (string, bool) {
	relPath, err := filepath.Rel(parent, child)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", false
	}
	return relPath, true
}

func collapseIgnores(ignores []string) []string {
	// Sort paths by shallowest to deepest.
	sort.Slice(ignores, func(i, j int) bool {
		depth := func(path string) int {
			return strings.Count(path, string(filepath.Separator))
		}
		return depth(ignores[i]) < depth(ignores[j])
	})

	var collapsedIgnores []string
	for _, ignore := range ignores {
		found := false
		// See if this ignore falls under any that have already been added.
		for _, parentIgnore := range collapsedIgnores {
			if _, ok := getSubpath(parentIgnore, ignore); ok {
				// We don't need to add this one since a parent or duplicate has
				// already been added.
				found = true
				break
			}
		}

		if !found {
			collapsedIgnores = append(collapsedIgnores, ignore)
		}
	}
	return collapsedIgnores
}

func rulesToIncludePath(path string) (rules []string) {
	for {
		rules = append(rules,
			// Include the path.
			fmt.Sprintf("!/%s", path),
		)

		parent := filepath.Dir(path)
		if parent == "." {
			return rules
		}

		// Exclude all other files in the parent directory (except for the
		// parent directory itself).
		rules = append(rules, fmt.Sprintf("/%s/", parent))
		path = parent
	}
}
