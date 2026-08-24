package gojaprotobuf

import (
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
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
		Name:    proto.String("cover_base.proto"),
		Package: proto.String("coverpkg"),
		Syntax:  proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Cover"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("value"),
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
			Name:     proto.String("extra"),
			Extendee: proto.String(".coverpkg.Cover"),
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
		Name:    proto.String("cover_base.proto"),
		Package: proto.String("coverpkg"),
		Syntax:  proto.String("proto2"),
		Options: &descriptorpb.FileOptions{
			UninterpretedOption: []*descriptorpb.UninterpretedOption{{
				Name: []*descriptorpb.UninterpretedOption_NamePart{{
					NamePart:    proto.String("buf.fake.additive.option"),
					IsExtension: proto.Bool(true),
				}},
				IdentifierValue: proto.String("anything"),
			}},
		},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Cover"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("value"),
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
			Name:     proto.String("extra"),
			Extendee: proto.String(".coverpkg.Cover"),
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
					Name:    proto.String("cover_absent.proto"),
					Package: proto.String("coverpkg"),
					Syntax:  proto.String("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: proto.String("Cover"),
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
					Name:    proto.String("cover_base.proto"),
					Package: proto.String("coverpkg.renamed"),
					Syntax:  proto.String("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: proto.String("Cover"),
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
					Name:    proto.String("cover_base.proto"),
					Package: proto.String("coverpkg"),
					Syntax:  proto.String("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{
						{Name: proto.String("Cover")},
						// Brand-new symbol the base does not have.
						{Name: proto.String("Invented")},
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
					Name:    proto.String("cover_base.proto"),
					Package: proto.String("coverpkg"),
					Syntax:  proto.String("proto3"),
					Service: []*descriptorpb.ServiceDescriptorProto{{
						// Same full name as the base MESSAGE "Cover" but a SERVICE.
						Name: proto.String("Cover"),
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
					Name:    proto.String("cover_base.proto"),
					Package: proto.String("coverpkg"),
					Syntax:  proto.String("proto2"),
					MessageType: []*descriptorpb.DescriptorProto{{
						Name: proto.String("Cover"),
						Field: []*descriptorpb.FieldDescriptorProto{{
							Name:   proto.String("value"),
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
						Name:     proto.String("extra"),
						Extendee: proto.String(".coverpkg.Cover"),
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
			wantSub: "field 101",
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
				NamePart:    proto.String("cover.test.additive"),
				IsExtension: proto.Bool(true),
			}},
			IdentifierValue: proto.String("v"),
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
		&descriptorpb.DescriptorProto{Name: proto.String("BrandNewType")},
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

// TestCoversFieldKindIsCoarse documents the known, accepted limitation that
// covers() compares fields only by coarse kind ("field"), so a field whose
// scalar TYPE changes (string -> int64) between two name+kind-equal base-owned
// files is NOT detected. This is acceptable because a base-owned file is, by
// definition, authoritative: the base registry is the source of truth and the
// incoming copy is discarded on a pass, so the base field type always wins. The
// gate's job is to catch redefinitions that introduce symbols the base lacks or
// change a symbol's KIND (message vs enum vs service), not to re-validate base
// internals. This test pins the behavior so a future tightening (e.g. comparing
// field types) is a deliberate, visible change.
func TestCoversFieldKindIsCoarse(t *testing.T) {
	baseFile := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    proto.String("field_kind_base.proto"),
		Package: proto.String("fieldkindpkg"),
		Syntax:  proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("amount"),
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

	// Same path, package, message + field name + kind, but the field's scalar
	// TYPE is int64 instead of string. covers() admits it (kind "field" matches).
	typeChanged := mustDescriptorFile(t, &descriptorpb.FileDescriptorProto{
		Name:    proto.String("field_kind_base.proto"),
		Package: proto.String("fieldkindpkg"),
		Syntax:  proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Holder"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("amount"),
				Number: proto.Int32(1),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
			}},
		}},
	}, nil)
	other, err := descriptorFileGraph(typeChanged)
	if err != nil {
		t.Fatalf("build type-changed graph: %v", err)
	}
	if err := base.covers(other); err != nil {
		t.Fatalf("covers rejected a field-type-only change; current contract admits it: %v", err)
	}
	// The base field type wins in production because the incoming file is
	// discarded on a pass. Confirm the base descriptor still reports string.
	baseHolder, ok := base.symbols["fieldkindpkg.Holder"]
	if !ok {
		t.Fatal("base graph missing fieldkindpkg.Holder")
	}
	msg, ok := baseHolder.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("fieldkindpkg.Holder kind = %T, want MessageDescriptor", baseHolder)
	}
	if ft := msg.Fields().ByName("amount").Kind(); ft != protoreflect.StringKind {
		t.Fatalf("base amount field kind = %v; base authority should keep StringKind", ft)
	}
}
