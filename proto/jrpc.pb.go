// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.17.3
// source: jrpc.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type Chain struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Title   string `protobuf:"bytes,1,opt,name=title,proto3" json:"title,omitempty"`
	Prefix  string `protobuf:"bytes,2,opt,name=prefix,proto3" json:"prefix,omitempty"`
	EsHost  string `protobuf:"bytes,3,opt,name=esHost,proto3" json:"esHost,omitempty"`
	Symbol  string `protobuf:"bytes,4,opt,name=symbol,proto3" json:"symbol,omitempty"`
	AppName string `protobuf:"bytes,5,opt,name=appName,proto3" json:"appName,omitempty"`
}

func (x *Chain) Reset() {
	*x = Chain{}
	if protoimpl.UnsafeEnabled {
		mi := &file_jrpc_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Chain) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Chain) ProtoMessage() {}

func (x *Chain) ProtoReflect() protoreflect.Message {
	mi := &file_jrpc_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Chain.ProtoReflect.Descriptor instead.
func (*Chain) Descriptor() ([]byte, []int) {
	return file_jrpc_proto_rawDescGZIP(), []int{0}
}

func (x *Chain) GetTitle() string {
	if x != nil {
		return x.Title
	}
	return ""
}

func (x *Chain) GetPrefix() string {
	if x != nil {
		return x.Prefix
	}
	return ""
}

func (x *Chain) GetEsHost() string {
	if x != nil {
		return x.EsHost
	}
	return ""
}

func (x *Chain) GetSymbol() string {
	if x != nil {
		return x.Symbol
	}
	return ""
}

func (x *Chain) GetAppName() string {
	if x != nil {
		return x.AppName
	}
	return ""
}

type JRPCConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Host        string   `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	Chain       []*Chain `protobuf:"bytes,2,rep,name=chain,proto3" json:"chain,omitempty"`
	WhiteList   []string `protobuf:"bytes,3,rep,name=whiteList,proto3" json:"whiteList,omitempty"`
	Name        string   `protobuf:"bytes,4,opt,name=name,proto3" json:"name,omitempty"`
	SwaggerHost string   `protobuf:"bytes,5,opt,name=swaggerHost,proto3" json:"swaggerHost,omitempty"`
}

func (x *JRPCConfig) Reset() {
	*x = JRPCConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_jrpc_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *JRPCConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*JRPCConfig) ProtoMessage() {}

func (x *JRPCConfig) ProtoReflect() protoreflect.Message {
	mi := &file_jrpc_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use JRPCConfig.ProtoReflect.Descriptor instead.
func (*JRPCConfig) Descriptor() ([]byte, []int) {
	return file_jrpc_proto_rawDescGZIP(), []int{1}
}

func (x *JRPCConfig) GetHost() string {
	if x != nil {
		return x.Host
	}
	return ""
}

func (x *JRPCConfig) GetChain() []*Chain {
	if x != nil {
		return x.Chain
	}
	return nil
}

func (x *JRPCConfig) GetWhiteList() []string {
	if x != nil {
		return x.WhiteList
	}
	return nil
}

func (x *JRPCConfig) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *JRPCConfig) GetSwaggerHost() string {
	if x != nil {
		return x.SwaggerHost
	}
	return ""
}

var File_jrpc_proto protoreflect.FileDescriptor

var file_jrpc_proto_rawDesc = []byte{
	0x0a, 0x0a, 0x6a, 0x72, 0x70, 0x63, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x05, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0x7f, 0x0a, 0x05, 0x43, 0x68, 0x61, 0x69, 0x6e, 0x12, 0x14, 0x0a, 0x05,
	0x74, 0x69, 0x74, 0x6c, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x69, 0x74,
	0x6c, 0x65, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x12, 0x16, 0x0a, 0x06, 0x65, 0x73,
	0x48, 0x6f, 0x73, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x65, 0x73, 0x48, 0x6f,
	0x73, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x73, 0x79, 0x6d, 0x62, 0x6f, 0x6c, 0x18, 0x04, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x06, 0x73, 0x79, 0x6d, 0x62, 0x6f, 0x6c, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x70,
	0x70, 0x4e, 0x61, 0x6d, 0x65, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x70, 0x70,
	0x4e, 0x61, 0x6d, 0x65, 0x22, 0x98, 0x01, 0x0a, 0x0a, 0x4a, 0x52, 0x50, 0x43, 0x43, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x12, 0x22, 0x0a, 0x05, 0x63, 0x68, 0x61, 0x69, 0x6e,
	0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x0c, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x2e, 0x43,
	0x68, 0x61, 0x69, 0x6e, 0x52, 0x05, 0x63, 0x68, 0x61, 0x69, 0x6e, 0x12, 0x1c, 0x0a, 0x09, 0x77,
	0x68, 0x69, 0x74, 0x65, 0x4c, 0x69, 0x73, 0x74, 0x18, 0x03, 0x20, 0x03, 0x28, 0x09, 0x52, 0x09,
	0x77, 0x68, 0x69, 0x74, 0x65, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x20, 0x0a,
	0x0b, 0x73, 0x77, 0x61, 0x67, 0x67, 0x65, 0x72, 0x48, 0x6f, 0x73, 0x74, 0x18, 0x05, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0b, 0x73, 0x77, 0x61, 0x67, 0x67, 0x65, 0x72, 0x48, 0x6f, 0x73, 0x74, 0x42,
	0x0a, 0x5a, 0x08, 0x2e, 0x2e, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x33,
}

var (
	file_jrpc_proto_rawDescOnce sync.Once
	file_jrpc_proto_rawDescData = file_jrpc_proto_rawDesc
)

func file_jrpc_proto_rawDescGZIP() []byte {
	file_jrpc_proto_rawDescOnce.Do(func() {
		file_jrpc_proto_rawDescData = protoimpl.X.CompressGZIP(file_jrpc_proto_rawDescData)
	})
	return file_jrpc_proto_rawDescData
}

var file_jrpc_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_jrpc_proto_goTypes = []interface{}{
	(*Chain)(nil),      // 0: proto.Chain
	(*JRPCConfig)(nil), // 1: proto.JRPCConfig
}
var file_jrpc_proto_depIdxs = []int32{
	0, // 0: proto.JRPCConfig.chain:type_name -> proto.Chain
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_jrpc_proto_init() }
func file_jrpc_proto_init() {
	if File_jrpc_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_jrpc_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Chain); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_jrpc_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*JRPCConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_jrpc_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_jrpc_proto_goTypes,
		DependencyIndexes: file_jrpc_proto_depIdxs,
		MessageInfos:      file_jrpc_proto_msgTypes,
	}.Build()
	File_jrpc_proto = out.File
	file_jrpc_proto_rawDesc = nil
	file_jrpc_proto_goTypes = nil
	file_jrpc_proto_depIdxs = nil
}