package gojaprotobuf

import (
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestCoversAcceptsAdditiveMetadataAndRejectsRedefinition is the dedicated unit
// test for [descriptorGraph.covers] — the semantic coverage gate that lets an
// incoming descriptor file whose bytes differ from the base only by additive,
// non-semantic metadata (the buf FileOptions extension ranges injected into
// google/protobuf/descriptor.proto) be satisfied from the base registry, while
// still rejecting a genuine redefinition. It exercises the method directly on
// hand-built descriptorGraph instances, covering both the accept and every
// reject branch: missing file path, package drift, missing symbol, symbol kind
// change, missing extension, and extension name change.
func TestCoversAcceptsAdditiveMetadataAndRejectsRedefinition(t *testing.T) {
	// Build a "base" file: pkg "coverpkg", message Cover, a field, and an
	// extension on Cover. The graph is the canonical base identity.
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("cover_base.proto"),
		Package: new("coverpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Cover"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("value"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("extra"),
			Extendee: new(".coverpkg.Cover"),
			Number:   proto.Int32(100),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
		}},
	}, nil)
	base, err := descriptorFileGraph(baseFile)
	if err != nil {
		t.Fatalf("build base graph: %v", err)
	}

	// A resolver that can build the "incoming" variants against the same base
	// symbols (needed when an incoming file imports a path the base owns).
	deps := new(protoregistry.Files)
	if err := deps.RegisterFile(baseFile); err != nil {
		t.Fatal(err)
	}

	// ACCEPT: an additive redeclaration of the same path whose only difference
	// is an extra FileOptions blob (the buf descriptor.proto case). Same path,
	// same package, same message + field + extension, so covers() returns nil.
	// The file is byte-different but semantically identical — this is the core
	// admission the gate exists to permit.
	covered, err := descriptorFileGraph(mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("cover_base.proto"),
		Package: new("coverpkg"),
		Syntax:  new("proto2"),
		Options: &descriptorpb.FileOptions{
			UninterpretedOption: []*descriptorpb.UninterpretedOption{{
				Name: []*descriptorpb.UninterpretedOption_NamePart{{
					NamePart:    new("buf.fake.additive.option"),
					IsExtension: new(true),
				}},
				IdentifierValue: new("anything"),
			}},
		},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Cover"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("value"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("extra"),
			Extendee: new(".coverpkg.Cover"),
			Number:   proto.Int32(100),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
		}},
	}, deps))
	if err != nil {
		t.Fatalf("build additive graph: %v", err)
	}
	// Prove the two graphs are NOT byte-identical (the whole reason covers()
	// exists instead of a proto.Equal fast path) yet ARE covered.
	if proto.Equal(
		protodesc.ToFileDescriptorProto(baseFile),
		protodesc.ToFileDescriptorProto(covered.files["cover_base.proto"]),
	) {
		t.Fatal("precondition: additive file should be byte-different from base")
	}
	if err := base.covers(covered); err != nil {
		t.Fatalf("covers accepted additive metadata = error, want nil: %v", err)
	}

	// Reject branches. Each builds an "incoming" graph that differs from the
	// base in exactly one semantic way and asserts covers() names the conflict.
	rejectCases := []struct {
		name    string
		build   func(*testing.T) *descriptorGraph
		wantSub string
	}{
		{
			name: "missing file path",
			build: func(t *testing.T) *descriptorGraph {
				other := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("cover_absent.proto"),
					Package: new("coverpkg"),
					Syntax:  new("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: new("Cover"),
					}},
				}, nil)
				g, err := descriptorFileGraph(other)
				if err != nil {
					t.Fatalf("build graph: %v", err)
				}
				return g
			},
			wantSub: "cover_absent.proto",
		},
		{
			name: "package drift",
			build: func(t *testing.T) *descriptorGraph {
				other := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("cover_base.proto"),
					Package: new("coverpkg.renamed"),
					Syntax:  new("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: new("Cover"),
					}},
				}, nil)
				g, err := descriptorFileGraph(other)
				if err != nil {
					t.Fatalf("build graph: %v", err)
				}
				return g
			},
			wantSub: "package changed",
		},
		{
			name: "missing symbol (genuine redefinition)",
			build: func(t *testing.T) *descriptorGraph {
				other := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("cover_base.proto"),
					Package: new("coverpkg"),
					Syntax:  new("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: new("Cover")},
						// Brand-new symbol the base does not have.
						{Name: new("Invented")},
					},
				}, nil)
				g, err := descriptorFileGraph(other)
				if err != nil {
					t.Fatalf("build graph: %v", err)
				}
				return g
			},
			wantSub: "coverpkg.Invented",
		},
		{
			// Isolate the symbol-kind-change branch. The incoming file reuses the
			// base symbol name "coverpkg.Cover" but changes its KIND from message
			// to service, and — critically — introduces NO new symbols that the
			// base lacks (a service with zero methods declares only the service
			// itself). This keeps the verdict deterministic regardless of map
			// iteration order: the only violation covers() can report is the kind
			// change. (A message-becomes-enum change, by contrast, also adds the
			// enum value as a fresh symbol, conflating two violations.)
			name: "symbol kind change (message becomes service)",
			build: func(t *testing.T) *descriptorGraph {
				other := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("cover_base.proto"),
					Package: new("coverpkg"),
					Syntax:  new("proto2"),
					Service: []*descriptorpb.ServiceDescriptorProto{{
						// Same full name as the base MESSAGE "Cover" but a SERVICE.
						Name: new("Cover"),
					}},
				}, nil)
				g, err := descriptorFileGraph(other)
				if err != nil {
					t.Fatalf("build graph: %v", err)
				}
				return g
			},
			wantSub: "changed kind",
		},
		{
			// The extension loop's key is (containing message, number). A
			// brand-new extension NAME would be caught first by the symbol
			// loop (every extension is also a symbol). To isolate the
			// extension-key branch we reuse the SAME extension full name
			// ("coverpkg.extra", so the symbol loop passes) but change its
			// number (100 -> 101), so the (Cover, 101) key is absent from base
			// while the symbol is present.
			name: "extension number mismatch",
			build: func(t *testing.T) *descriptorGraph {
				other := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
					Name:    new("cover_base.proto"),
					Package: new("coverpkg"),
					Syntax:  new("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: new("Cover"),
						Field: []*descriptorpb.FieldDescriptorProto{{
							Name:   new("value"),
							Number: proto.Int32(1),
							Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						}},
						ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
							Start: proto.Int32(100),
							End:   proto.Int32(201),
						}},
					}},
					Extension: []*descriptorpb.FieldDescriptorProto{{
						// Same full name as the base extension, but number 101
						// (base registers it at 100) — a different key.
						Name:     new("extra"),
						Extendee: new(".coverpkg.Cover"),
						Number:   proto.Int32(101),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
					}},
				}, nil)
				g, err := descriptorFileGraph(other)
				if err != nil {
					t.Fatalf("build graph: %v", err)
				}
				return g
			},
			wantSub: "number changed from 100 to 101",
		},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			other := tc.build(t)
			err := base.covers(other)
			if err == nil {
				t.Fatalf("covers accepted %s; want a non-nil error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("covers(%s) error = %q; want substring %q", tc.name, err.Error(), tc.wantSub)
			}
		})
	}

	// Reflexivity: base covers itself (trivially — same identity). This guards
	// against any branch that would spuriously reject an identical graph.
	if err := base.covers(base); err != nil {
		t.Fatalf("covers(base, base) = %v; want nil", err)
	}
}

// TestInstallFileProtosBaseOwnedSemanticCoverage proves the covers() gate works
// end-to-end through the public installFileProtos path: loading a descriptor set
// whose only base-owned file is google/protobuf/timestamp.proto, augmented with
// additive FileOptions metadata (mirroring how buf augments descriptor.proto),
// is accepted, while the SAME path with a genuinely new symbol is rejected. This
// is the module-level proof that the production descriptor-loading path admits
// the buf-augmented base-owned file and rejects a real redefinition.
func TestInstallFileProtosBaseOwnedSemanticCoverage(t *testing.T) {
	// Base registry owns google/protobuf/timestamp.proto (from the well-known
	// descriptor), so a dynamic load of that path is base-owned.
	baseFiles := new(protoregistry.Files)
	if err := baseFiles.RegisterFile(timestamppb.File_google_protobuf_timestamp_proto); err != nil {
		t.Fatal(err)
	}
	module, err := New(
		goja.New(),
		WithResolver(new(protoregistry.Types)),
		WithFiles(baseFiles),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tsPath := timestamppb.File_google_protobuf_timestamp_proto.Path()

	// Build a base-owned FileDescriptorProto that is byte-different (extra
	// additive FileOptions metadata) but semantically identical to the base.
	augmented := protodesc.ToFileDescriptorProto(
		timestamppb.File_google_protobuf_timestamp_proto,
	)
	augmented.Options = &descriptorpb.FileOptions{
		UninterpretedOption: []*descriptorpb.UninterpretedOption{{
			Name: []*descriptorpb.UninterpretedOption_NamePart{{
				NamePart:    new("cover.test.additive"),
				IsExtension: new(true),
			}},
			IdentifierValue: new("v"),
		}},
	}
	augmentedBytes, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{augmented},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Precondition: it is genuinely byte-different and would have been rejected
	// by a naive byte-compare gate. It loads a single base-owned file only.
	if names, err := module.loadDescriptorSetBytes(augmentedBytes); err != nil {
		t.Fatalf("augmented base-owned file %s rejected: %v", tsPath, err)
	} else if len(names) != 0 {
		t.Fatalf("augmented base-owned file registered local names %v; want none (served from base)", names)
	}
	// The base identity is unchanged: Timestamp resolves from base.
	if _, err := module.FindDescriptor("google.protobuf.Timestamp"); err != nil {
		t.Fatalf("Timestamp no longer resolvable after augmented load: %v", err)
	}

	// Now build the SAME path with a genuinely NEW symbol the base lacks: a real
	// redefinition that the semantic gate must reject.
	redefining := protodesc.ToFileDescriptorProto(
		timestamppb.File_google_protobuf_timestamp_proto,
	)
	redefining.MessageType = append(
		redefining.MessageType,
		&descriptorpb.DescriptorProto{Name: new("BrandNewType")},
	)
	redefiningBytes, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{redefining},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.loadDescriptorSetBytes(redefiningBytes); err == nil {
		t.Fatalf("redefining base-owned file %s accepted; want a conflict error", tsPath)
	}
	// And the new symbol was NOT published (transactional rollback).
	if _, err := module.FindDescriptor("google.protobuf.BrandNewType"); err == nil {
		t.Fatal("rejected redefinition published its new symbol")
	}
}

// TestCoversRejectsStructuralFieldDifferences proves covers() rejects field-level
// incompatibilities: scalar type changes, number changes, and cardinality changes.
func TestCoversRejectsStructuralFieldDifferences(t *testing.T) {
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("field_struct_base.proto"),
		Package: new("fieldstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("amount"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	base, err := descriptorFileGraph(baseFile)
	if err != nil {
		t.Fatalf("build base graph: %v", err)
	}

	// 1. Scalar type change (string -> int64).
	typeChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("field_struct_base.proto"),
		Package: new("fieldstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("amount"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
			}},
		}},
	}, nil)
	otherType, err := descriptorFileGraph(typeChanged)
	if err != nil {
		t.Fatalf("build type-changed graph: %v", err)
	}
	if err := base.covers(otherType); err == nil || !strings.Contains(err.Error(), "type changed") {
		t.Fatalf("covers = %v, want error containing 'type changed'", err)
	}

	// 2. Field number change (1 -> 2).
	numberChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("field_struct_base.proto"),
		Package: new("fieldstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("amount"),
				Number: proto.Int32(2),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	otherNumber, err := descriptorFileGraph(numberChanged)
	if err != nil {
		t.Fatalf("build number-changed graph: %v", err)
	}
	if err := base.covers(otherNumber); err == nil || !strings.Contains(err.Error(), "not present in base registry") {
		t.Fatalf("covers = %v, want error containing 'not present in base registry'", err)
	}

	// 3. Cardinality change (optional -> repeated).
	cardinalityChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("field_struct_base.proto"),
		Package: new("fieldstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("amount"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	otherCard, err := descriptorFileGraph(cardinalityChanged)
	if err != nil {
		t.Fatalf("build cardinality-changed graph: %v", err)
	}
	if err := base.covers(otherCard); err == nil || !strings.Contains(err.Error(), "cardinality changed") {
		t.Fatalf("covers = %v, want error containing 'cardinality changed'", err)
	}
}

// TestCoversRejectsStructuralEnumDifferences proves covers() rejects enum value number mutations.
func TestCoversRejectsStructuralEnumDifferences(t *testing.T) {
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("enum_struct_base.proto"),
		Package: new("enumstructpkg"),
		Syntax:  new("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: new("Status"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: new("UNKNOWN"), Number: proto.Int32(0)},
				{Name: new("ACTIVE"), Number: proto.Int32(1)},
			},
		}},
	}, nil)
	base, err := descriptorFileGraph(baseFile)
	if err != nil {
		t.Fatalf("build base graph: %v", err)
	}

	// ACTIVE changed number from 1 to 2.
	changedFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("enum_struct_base.proto"),
		Package: new("enumstructpkg"),
		Syntax:  new("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: new("Status"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: new("UNKNOWN"), Number: proto.Int32(0)},
				{Name: new("ACTIVE"), Number: proto.Int32(2)},
			},
		}},
	}, nil)
	other, err := descriptorFileGraph(changedFile)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(other); err == nil || !strings.Contains(err.Error(), "number changed") {
		t.Fatalf("covers = %v, want error containing 'number changed'", err)
	}
}

// TestCoversRejectsStructuralServiceDifferences proves covers() rejects RPC signature mutations.
func TestCoversRejectsStructuralServiceDifferences(t *testing.T) {
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("service_struct_base.proto"),
		Package: new("servicestructpkg"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("ReqA")},
			{Name: new("ReqB")},
			{Name: new("Resp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: new("TestService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:            new("Call"),
				InputType:       new(".servicestructpkg.ReqA"),
				OutputType:      new(".servicestructpkg.Resp"),
				ClientStreaming: new(false),
				ServerStreaming: new(false),
			}},
		}},
	}, nil)
	base, err := descriptorFileGraph(baseFile)
	if err != nil {
		t.Fatalf("build base graph: %v", err)
	}

	// 1. Input type changed ReqA -> ReqB.
	inputChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("service_struct_base.proto"),
		Package: new("servicestructpkg"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("ReqA")},
			{Name: new("ReqB")},
			{Name: new("Resp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: new("TestService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:            new("Call"),
				InputType:       new(".servicestructpkg.ReqB"),
				OutputType:      new(".servicestructpkg.Resp"),
				ClientStreaming: new(false),
				ServerStreaming: new(false),
			}},
		}},
	}, nil)
	otherInput, err := descriptorFileGraph(inputChanged)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(otherInput); err == nil || !strings.Contains(err.Error(), "input type changed") {
		t.Fatalf("covers = %v, want error containing 'input type changed'", err)
	}

	// 2. Client streaming changed false -> true.
	streamingChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("service_struct_base.proto"),
		Package: new("servicestructpkg"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: new("ReqA")},
			{Name: new("ReqB")},
			{Name: new("Resp")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name: new("TestService"),
			Method: []*descriptorpb.MethodDescriptorProto{{
				Name:            new("Call"),
				InputType:       new(".servicestructpkg.ReqA"),
				OutputType:      new(".servicestructpkg.Resp"),
				ClientStreaming: new(true),
				ServerStreaming: new(false),
			}},
		}},
	}, nil)
	otherStream, err := descriptorFileGraph(streamingChanged)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(otherStream); err == nil || !strings.Contains(err.Error(), "client streaming changed") {
		t.Fatalf("covers = %v, want error containing 'client streaming changed'", err)
	}
}

// TestCoversRejectsStructuralExtensionDifferences proves covers() rejects extension type and number mutations.
func TestCoversRejectsStructuralExtensionDifferences(t *testing.T) {
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("ext_struct_base.proto"),
		Package: new("extstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Target"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("opt"),
			Extendee: new(".extstructpkg.Target"),
			Number:   proto.Int32(100),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
		}},
	}, nil)
	base, err := descriptorFileGraph(baseFile)
	if err != nil {
		t.Fatalf("build base graph: %v", err)
	}

	// Extension type changed int64 -> string.
	typeChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("ext_struct_base.proto"),
		Package: new("extstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Target"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("opt"),
			Extendee: new(".extstructpkg.Target"),
			Number:   proto.Int32(100),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
		}},
	}, nil)
	otherType, err := descriptorFileGraph(typeChanged)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(otherType); err == nil || !strings.Contains(err.Error(), "type changed") {
		t.Fatalf("covers = %v, want error containing 'type changed'", err)
	}

	// Extension number changed 100 -> 101.
	numChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("ext_struct_base.proto"),
		Package: new("extstructpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Target"),
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     new("opt"),
			Extendee: new(".extstructpkg.Target"),
			Number:   proto.Int32(101),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
		}},
	}, nil)
	otherNum, err := descriptorFileGraph(numChanged)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(otherNum); err == nil || !strings.Contains(err.Error(), "number changed") {
		t.Fatalf("covers = %v, want error containing 'number changed'", err)
	}
}

// TestCoversRejectsSyntaxAndRelocationDifferences proves covers() rejects syntax changes and symbol relocations.
func TestCoversRejectsSyntaxAndRelocationDifferences(t *testing.T) {
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("syntax_base.proto"),
		Package: new("syntaxpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("val"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	base, err := descriptorFileGraph(baseFile)
	if err != nil {
		t.Fatalf("build base graph: %v", err)
	}

	// Syntax changed proto2 -> proto3.
	syntaxChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("syntax_base.proto"),
		Package: new("syntaxpkg"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("val"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	otherSyntax, err := descriptorFileGraph(syntaxChanged)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(otherSyntax); err == nil || !strings.Contains(err.Error(), "syntax changed") {
		t.Fatalf("covers = %v, want error containing 'syntax changed'", err)
	}

	// Symbol Msg relocated to a different file path.
	relocated := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    new("relocated.proto"),
		Package: new("syntaxpkg"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("val"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}, nil)
	otherRelocated, err := descriptorFileGraph(relocated)
	if err != nil {
		t.Fatalf("build other graph: %v", err)
	}
	if err := base.covers(otherRelocated); err == nil || !strings.Contains(err.Error(), "not present in the base registry") {
		t.Fatalf("covers = %v, want error containing 'not present in the base registry'", err)
	}
}

// TestCoversRejectsExtensionSemanticChange reproduces a coverage hole in the
// compareDescriptors FieldDescriptor branch: a top-level extension's target
// message type, target enum type, and packed attribute were not compared, so an
// incoming descriptor could retarget an extension (same full name, number, kind,
// cardinality, and extendee) to a different message or enum type — or flip its
// packing — and still be accepted by covers(). Each subtest mutates exactly one
// extension property and asserts the gate now rejects it.
func TestCoversRejectsExtensionSemanticChange(t *testing.T) {
	// assertReject fails if the base graph accepts the incoming graph.
	assertReject := func(t *testing.T, base, incoming *descriptorGraph) {
		t.Helper()
		if err := base.covers(incoming); err == nil {
			t.Fatal("covers accepted the extension semantic change; want a non-nil error")
		}
	}

	t.Run("message target change", func(t *testing.T) {
		baseFD := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
			Name:    new("cover_ext_semantic.proto"),
			Package: new("coverpkg"),
			Syntax:  new("proto2"),
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: new("Cover"),
					ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
						Start: proto.Int32(100),
						End:   proto.Int32(200),
					}},
				},
				{Name: new("TargetA")},
				{Name: new("TargetB")},
			},
			Extension: []*descriptorpb.FieldDescriptorProto{{
				Name:     new("ext"),
				Extendee: new(".coverpkg.Cover"),
				Number:   proto.Int32(100),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: new(".coverpkg.TargetA"),
			}},
		}, nil)
		base, err := descriptorFileGraph(baseFD)
		if err != nil {
			t.Fatalf("build base graph: %v", err)
		}
		// Incoming retargets the extension to TargetB: same full name, number,
		// kind (MessageKind), cardinality, and extendee — only the target type
		// differs, which the FieldDescriptor branch used to ignore.
		incomingFD := protodesc.ToFileDescriptorProto(baseFD)
		incomingFD.Extension[0].TypeName = new(".coverpkg.TargetB")
		incoming, err := descriptorFileGraph(mustDescriptorFile(t, incomingFD, nil))
		if err != nil {
			t.Fatalf("build incoming graph: %v", err)
		}
		assertReject(t, base, incoming)
	})

	t.Run("enum target change", func(t *testing.T) {
		baseFD := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
			Name:    new("cover_ext_enum.proto"),
			Package: new("coverpkg"),
			Syntax:  new("proto2"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: new("Cover"),
				ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
					Start: proto.Int32(100),
					End:   proto.Int32(200),
				}},
			}},
			EnumType: []*descriptorpb.EnumDescriptorProto{
				{
					Name: new("EnumA"),
					Value: []*descriptorpb.EnumValueDescriptorProto{
						{Name: new("EA_ZERO"), Number: proto.Int32(0)},
					},
				},
				{
					Name: new("EnumB"),
					Value: []*descriptorpb.EnumValueDescriptorProto{
						{Name: new("EB_ZERO"), Number: proto.Int32(0)},
					},
				},
			},
			Extension: []*descriptorpb.FieldDescriptorProto{{
				Name:     new("ext"),
				Extendee: new(".coverpkg.Cover"),
				Number:   proto.Int32(100),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
				TypeName: new(".coverpkg.EnumA"),
			}},
		}, nil)
		base, err := descriptorFileGraph(baseFD)
		if err != nil {
			t.Fatalf("build base graph: %v", err)
		}
		incomingFD := protodesc.ToFileDescriptorProto(baseFD)
		incomingFD.Extension[0].TypeName = new(".coverpkg.EnumB")
		incoming, err := descriptorFileGraph(mustDescriptorFile(t, incomingFD, nil))
		if err != nil {
			t.Fatalf("build incoming graph: %v", err)
		}
		assertReject(t, base, incoming)
	})

	t.Run("packed attribute change", func(t *testing.T) {
		baseFD := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
			Name:    new("cover_ext_packed.proto"),
			Package: new("coverpkg"),
			Syntax:  new("proto2"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: new("Cover"),
				ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
					Start: proto.Int32(100),
					End:   proto.Int32(200),
				}},
			}},
			Extension: []*descriptorpb.FieldDescriptorProto{{
				Name:     new("ext"),
				Extendee: new(".coverpkg.Cover"),
				Number:   proto.Int32(100),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Options:  &descriptorpb.FieldOptions{Packed: new(true)},
			}},
		}, nil)
		base, err := descriptorFileGraph(baseFD)
		if err != nil {
			t.Fatalf("build base graph: %v", err)
		}
		// Incoming flips packing off: same full name, number, kind, and
		// cardinality, differing only in the packed attribute.
		incomingFD := protodesc.ToFileDescriptorProto(baseFD)
		incomingFD.Extension[0].Options.Packed = new(false)
		incoming, err := descriptorFileGraph(mustDescriptorFile(t, incomingFD, nil))
		if err != nil {
			t.Fatalf("build incoming graph: %v", err)
		}
		assertReject(t, base, incoming)
	})
}
