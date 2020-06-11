// Code generated by protoc-gen-go. DO NOT EDIT.
// source: _proto/blimp/errors/v0/errors.proto

package errors

import (
	fmt "fmt"
	proto "github.com/golang/protobuf/proto"
	math "math"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

// Error is a union of possible error types. Each field corresponds to a type
// in our Go code.
type Error struct {
	ContextError  *ContextError  `protobuf:"bytes,1,opt,name=context_error,json=contextError,proto3" json:"context_error,omitempty"`
	FriendlyError *FriendlyError `protobuf:"bytes,2,opt,name=friendly_error,json=friendlyError,proto3" json:"friendly_error,omitempty"`
	// `text` is the default case. If none of the above fields are defined, then
	// it's assumed that the error isn't a special Blimp type, and can be created
	// with errors.New.
	Text                 string   `protobuf:"bytes,4,opt,name=text,proto3" json:"text,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *Error) Reset()         { *m = Error{} }
func (m *Error) String() string { return proto.CompactTextString(m) }
func (*Error) ProtoMessage()    {}
func (*Error) Descriptor() ([]byte, []int) {
	return fileDescriptor_634bedf48a53d953, []int{0}
}

func (m *Error) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_Error.Unmarshal(m, b)
}
func (m *Error) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_Error.Marshal(b, m, deterministic)
}
func (m *Error) XXX_Merge(src proto.Message) {
	xxx_messageInfo_Error.Merge(m, src)
}
func (m *Error) XXX_Size() int {
	return xxx_messageInfo_Error.Size(m)
}
func (m *Error) XXX_DiscardUnknown() {
	xxx_messageInfo_Error.DiscardUnknown(m)
}

var xxx_messageInfo_Error proto.InternalMessageInfo

func (m *Error) GetContextError() *ContextError {
	if m != nil {
		return m.ContextError
	}
	return nil
}

func (m *Error) GetFriendlyError() *FriendlyError {
	if m != nil {
		return m.FriendlyError
	}
	return nil
}

func (m *Error) GetText() string {
	if m != nil {
		return m.Text
	}
	return ""
}

type ContextError struct {
	Error                *Error   `protobuf:"bytes,1,opt,name=error,proto3" json:"error,omitempty"`
	Context              string   `protobuf:"bytes,2,opt,name=context,proto3" json:"context,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *ContextError) Reset()         { *m = ContextError{} }
func (m *ContextError) String() string { return proto.CompactTextString(m) }
func (*ContextError) ProtoMessage()    {}
func (*ContextError) Descriptor() ([]byte, []int) {
	return fileDescriptor_634bedf48a53d953, []int{1}
}

func (m *ContextError) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_ContextError.Unmarshal(m, b)
}
func (m *ContextError) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_ContextError.Marshal(b, m, deterministic)
}
func (m *ContextError) XXX_Merge(src proto.Message) {
	xxx_messageInfo_ContextError.Merge(m, src)
}
func (m *ContextError) XXX_Size() int {
	return xxx_messageInfo_ContextError.Size(m)
}
func (m *ContextError) XXX_DiscardUnknown() {
	xxx_messageInfo_ContextError.DiscardUnknown(m)
}

var xxx_messageInfo_ContextError proto.InternalMessageInfo

func (m *ContextError) GetError() *Error {
	if m != nil {
		return m.Error
	}
	return nil
}

func (m *ContextError) GetContext() string {
	if m != nil {
		return m.Context
	}
	return ""
}

type FriendlyError struct {
	FriendlyMessage      string   `protobuf:"bytes,1,opt,name=friendly_message,json=friendlyMessage,proto3" json:"friendly_message,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

func (m *FriendlyError) Reset()         { *m = FriendlyError{} }
func (m *FriendlyError) String() string { return proto.CompactTextString(m) }
func (*FriendlyError) ProtoMessage()    {}
func (*FriendlyError) Descriptor() ([]byte, []int) {
	return fileDescriptor_634bedf48a53d953, []int{2}
}

func (m *FriendlyError) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_FriendlyError.Unmarshal(m, b)
}
func (m *FriendlyError) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_FriendlyError.Marshal(b, m, deterministic)
}
func (m *FriendlyError) XXX_Merge(src proto.Message) {
	xxx_messageInfo_FriendlyError.Merge(m, src)
}
func (m *FriendlyError) XXX_Size() int {
	return xxx_messageInfo_FriendlyError.Size(m)
}
func (m *FriendlyError) XXX_DiscardUnknown() {
	xxx_messageInfo_FriendlyError.DiscardUnknown(m)
}

var xxx_messageInfo_FriendlyError proto.InternalMessageInfo

func (m *FriendlyError) GetFriendlyMessage() string {
	if m != nil {
		return m.FriendlyMessage
	}
	return ""
}

func init() {
	proto.RegisterType((*Error)(nil), "blimp.errors.v0.Error")
	proto.RegisterType((*ContextError)(nil), "blimp.errors.v0.ContextError")
	proto.RegisterType((*FriendlyError)(nil), "blimp.errors.v0.FriendlyError")
}

func init() {
	proto.RegisterFile("_proto/blimp/errors/v0/errors.proto", fileDescriptor_634bedf48a53d953)
}

var fileDescriptor_634bedf48a53d953 = []byte{
	// 247 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0x52, 0x8e, 0x2f, 0x28, 0xca,
	0x2f, 0xc9, 0xd7, 0x4f, 0xca, 0xc9, 0xcc, 0x2d, 0xd0, 0x4f, 0x2d, 0x2a, 0xca, 0x2f, 0x2a, 0xd6,
	0x2f, 0x33, 0x80, 0xb2, 0xf4, 0xc0, 0x92, 0x42, 0xfc, 0x60, 0x59, 0x3d, 0xa8, 0x58, 0x99, 0x81,
	0xd2, 0x32, 0x46, 0x2e, 0x56, 0x57, 0x10, 0x4f, 0xc8, 0x89, 0x8b, 0x37, 0x39, 0x3f, 0xaf, 0x24,
	0xb5, 0xa2, 0x24, 0x1e, 0x2c, 0x2d, 0xc1, 0xa8, 0xc0, 0xa8, 0xc1, 0x6d, 0x24, 0xab, 0x87, 0xa6,
	0x45, 0xcf, 0x19, 0xa2, 0x0a, 0xac, 0x2b, 0x88, 0x27, 0x19, 0x89, 0x27, 0xe4, 0xca, 0xc5, 0x97,
	0x56, 0x94, 0x99, 0x9a, 0x97, 0x92, 0x53, 0x09, 0x35, 0x84, 0x09, 0x6c, 0x88, 0x1c, 0x86, 0x21,
	0x6e, 0x50, 0x65, 0x10, 0x53, 0x78, 0xd3, 0x90, 0xb9, 0x42, 0x42, 0x5c, 0x2c, 0x20, 0x33, 0x25,
	0x58, 0x14, 0x18, 0x35, 0x38, 0x83, 0xc0, 0x6c, 0xa5, 0x30, 0x2e, 0x1e, 0x64, 0x8b, 0x85, 0x74,
	0xb8, 0x58, 0x91, 0x9d, 0x29, 0x86, 0x61, 0x03, 0xc4, 0x64, 0x88, 0x22, 0x21, 0x09, 0x2e, 0x76,
	0xa8, 0x43, 0xc1, 0x2e, 0xe2, 0x0c, 0x82, 0x71, 0x95, 0xac, 0xb8, 0x78, 0x51, 0xdc, 0x22, 0xa4,
	0xc9, 0x25, 0x00, 0xf7, 0x43, 0x6e, 0x6a, 0x71, 0x71, 0x62, 0x7a, 0x2a, 0xd8, 0x0e, 0xce, 0x20,
	0x7e, 0x98, 0xb8, 0x2f, 0x44, 0xd8, 0x49, 0x33, 0x4a, 0x3d, 0x3d, 0xb3, 0x24, 0xa3, 0x34, 0x49,
	0x2f, 0x39, 0x3f, 0x57, 0x3f, 0x3b, 0x35, 0x27, 0x25, 0x11, 0x1a, 0xfc, 0x05, 0xd9, 0xe9, 0xfa,
	0x90, 0xe8, 0x80, 0x38, 0x28, 0x89, 0x0d, 0xcc, 0x33, 0x06, 0x04, 0x00, 0x00, 0xff, 0xff, 0x32,
	0x1e, 0x43, 0x2d, 0xa6, 0x01, 0x00, 0x00,
}