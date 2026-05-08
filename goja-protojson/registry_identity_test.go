package gojaprotojson_test

import (
	"encoding/json"
	"testing"

	"github.com/joeycumines/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	gojaprotojson "github.com/joeycumines/goja-protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestFilesOnlyGraphProtoJSONRoundTrip(t *testing.T) {
	runtime := goja.New()
	files := new(protoregistry.Files)
	if err := files.RegisterFile(filesOnlyProtoJSONGraph(t)); err != nil {
		t.Fatal(err)
	}
	protobuf, err := gojaprotobuf.New(
		runtime,
		gojaprotobuf.WithResolver(new(protoregistry.Types)),
		gojaprotobuf.WithFiles(files),
	)
	if err != nil {
		t.Fatal(err)
	}
	protobufExports := runtime.NewObject()
	if err := protobuf.SetupExports(protobufExports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("pb", protobufExports); err != nil {
		t.Fatal(err)
	}
	protoJSON, err := gojaprotojson.New(
		runtime,
		gojaprotojson.WithProtobuf(protobuf),
	)
	if err != nil {
		t.Fatal(err)
	}
	protoJSONExports := runtime.NewObject()
	if err := protoJSON.SetupExports(protoJSONExports); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("protojson", protoJSONExports); err != nil {
		t.Fatal(err)
	}

	value, err := runtime.RunString(`
		const Payload = pb.messageType("fileonly.Payload");
		const payload = new Payload();
		payload.set("value", "9223372036854775807");
		payload.set("mode", 1);
		const Envelope = pb.messageType("fileonly.Envelope");
		const envelope = new Envelope();
		envelope.set("payload", pb.anyPack(Payload, payload));
		envelope.set("created_at", pb.timestampFromMs(1700000000123));
		const envelopeJSON = protojson.marshal(envelope);
		const decodedEnvelope = protojson.unmarshal(
			"fileonly.Envelope",
			envelopeJSON,
		);
		const decodedPayload = pb.anyUnpack(
			decodedEnvelope.get("payload"),
			Payload,
		);

		const Host = pb.messageType("fileonly.Host");
		const host = new Host();
		host.set("[fileonly.extra]", "9223372036854775807");
		const hostJSON = protojson.marshal(host);
		const decodedHost = protojson.unmarshal("fileonly.Host", hostJSON);

		JSON.stringify({
			envelopeJSON,
			value: decodedPayload.get("value").toString(),
			mode: decodedPayload.get("mode"),
			timestampMillis: pb.timestampMs(
				decodedEnvelope.get("created_at"),
			),
			hostJSON,
			extra: decodedHost.get("[fileonly.extra]").toString(),
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		EnvelopeJSON    string `json:"envelopeJSON"`
		Value           string `json:"value"`
		Mode            int    `json:"mode"`
		TimestampMillis int64  `json:"timestampMillis"`
		HostJSON        string `json:"hostJSON"`
		Extra           string `json:"extra"`
	}
	if err := json.Unmarshal([]byte(value.String()), &result); err != nil {
		t.Fatal(err)
	}
	if result.Value != "9223372036854775807" {
		t.Fatalf("Any payload int64 = %q", result.Value)
	}
	if result.Mode != 1 {
		t.Fatalf("Any payload enum = %d", result.Mode)
	}
	if result.TimestampMillis != 1700000000123 {
		t.Fatalf("timestamp millis = %d", result.TimestampMillis)
	}
	if result.Extra != "9223372036854775807" {
		t.Fatalf("extension int64 = %q", result.Extra)
	}

	var envelope struct {
		Payload struct {
			Type  string `json:"@type"`
			Value string `json:"value"`
			Mode  string `json:"mode"`
		} `json:"payload"`
		CreatedAt string `json:"createdAt"`
	}
	if err := json.Unmarshal([]byte(result.EnvelopeJSON), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Payload.Type != "type.googleapis.com/fileonly.Payload" {
		t.Fatalf("Any type URL = %q", envelope.Payload.Type)
	}
	if envelope.Payload.Value != "9223372036854775807" {
		t.Fatalf("Any JSON int64 = %q", envelope.Payload.Value)
	}
	if envelope.Payload.Mode != "MODE_ACTIVE" {
		t.Fatalf("Any JSON enum = %q", envelope.Payload.Mode)
	}
	if envelope.CreatedAt != "2023-11-14T22:13:20.123Z" {
		t.Fatalf("Timestamp JSON = %q", envelope.CreatedAt)
	}

	var host map[string]string
	if err := json.Unmarshal([]byte(result.HostJSON), &host); err != nil {
		t.Fatal(err)
	}
	if host["[fileonly.extra]"] != "9223372036854775807" {
		t.Fatalf("extension JSON = %q", host["[fileonly.extra]"])
	}
}

func filesOnlyProtoJSONGraph(t *testing.T) protoreflect.FileDescriptor {
	t.Helper()
	dependencies := new(protoregistry.Files)
	for _, dependency := range []protoreflect.FileDescriptor{
		anypb.File_google_protobuf_any_proto,
		timestamppb.File_google_protobuf_timestamp_proto,
	} {
		if err := dependencies.RegisterFile(dependency); err != nil {
			t.Fatal(err)
		}
	}
	optional := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	message := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
	int64Type := descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum()
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:       proto.String("fileonly/root.proto"),
		Package:    proto.String("fileonly"),
		Syntax:     proto.String("proto2"),
		Dependency: []string{anypb.File_google_protobuf_any_proto.Path(), timestamppb.File_google_protobuf_timestamp_proto.Path()},
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: proto.String("Mode"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("MODE_UNSPECIFIED"), Number: proto.Int32(0)},
				{Name: proto.String("MODE_ACTIVE"), Number: proto.Int32(1)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Payload"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("value"),
						JsonName: proto.String("value"),
						Number:   proto.Int32(1),
						Label:    optional,
						Type:     int64Type,
					},
					{
						Name:     proto.String("mode"),
						JsonName: proto.String("mode"),
						Number:   proto.Int32(2),
						Label:    optional,
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: proto.String(".fileonly.Mode"),
					},
				},
			},
			{
				Name: proto.String("Envelope"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("payload"),
						JsonName: proto.String("payload"),
						Number:   proto.Int32(1),
						Label:    optional,
						Type:     message,
						TypeName: proto.String(".google.protobuf.Any"),
					},
					{
						Name:     proto.String("created_at"),
						JsonName: proto.String("createdAt"),
						Number:   proto.Int32(2),
						Label:    optional,
						Type:     message,
						TypeName: proto.String(".google.protobuf.Timestamp"),
					},
				},
			},
			{
				Name: proto.String("Host"),
				ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
					Start: proto.Int32(100),
					End:   proto.Int32(200),
				}},
			},
		},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     proto.String("extra"),
			JsonName: proto.String("extra"),
			Number:   proto.Int32(100),
			Label:    optional,
			Type:     int64Type,
			Extendee: proto.String(".fileonly.Host"),
		}},
	}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
