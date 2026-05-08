package gojagrpc

import (
	"strings"
	"testing"

	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
)

type existingReflectionServer struct {
	reflectionpb.UnimplementedServerReflectionServer
}

func TestEnableReflectionReturnsRegistrationCollision(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	reflectionpb.RegisterServerReflectionServer(
		env.channel,
		new(existingReflectionServer),
	)
	if err := env.grpcMod.EnableReflection(); err == nil ||
		!strings.Contains(err.Error(), "already registered") {
		t.Fatalf("EnableReflection collision = %v", err)
	}
	env.grpcMod.mu.Lock()
	reflectionSet := env.grpcMod.reflectionSet
	env.grpcMod.mu.Unlock()
	if reflectionSet {
		t.Fatal("failed reflection registration marked module enabled")
	}
}
