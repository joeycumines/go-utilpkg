package inprocgrpc

import (
	"fmt"

	"google.golang.org/grpc/codes"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func messageSize(message any) (int, error) {
	if value, ok := message.(proto.Message); ok {
		return proto.Size(value), nil
	}
	codec := getCodecV2(grpcproto.Name)
	if codec == nil {
		return 0, fmt.Errorf("no codec found for message size")
	}
	buffers, err := codec.Marshal(message)
	if err != nil {
		return 0, err
	}
	defer buffers.Free()
	return buffers.Len(), nil
}

func checkSendSize(message any, maximum int, configured bool) error {
	if !configured {
		return nil
	}
	size, err := messageSize(message)
	if err != nil {
		description, ok := describeRPCValue(err)
		if !ok {
			return internalFailureError("grpc message marshaling")
		}
		return status.Errorf(
			codes.Internal,
			"grpc: error while marshaling: %s",
			description,
		)
	}
	if size > maximum {
		return status.Errorf(
			codes.ResourceExhausted,
			"trying to send message larger than max (%d vs. %d)",
			size,
			maximum,
		)
	}
	return nil
}

func checkReceiveSize(message any, maximum int, configured bool) error {
	if !configured {
		return nil
	}
	size, err := messageSize(message)
	if err != nil {
		description, ok := describeRPCValue(err)
		if !ok {
			return internalFailureError("grpc message unmarshaling")
		}
		return status.Errorf(
			codes.Internal,
			"grpc: error while unmarshaling: %s",
			description,
		)
	}
	if size > maximum {
		return status.Errorf(
			codes.ResourceExhausted,
			"grpc: received message larger than max (%d vs. %d)",
			size,
			maximum,
		)
	}
	return nil
}
