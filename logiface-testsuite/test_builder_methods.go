package testsuite

import (
	"github.com/joeycumines/logiface"
	"testing"
)

// TestBuilderMethods is part of TestSuite; it tests that the logger implementation
// correctly handles the fluent builder methods Slice, Map, MapFields, and ArgFields
// across Builder, Context, Chain, ArrayBuilder, and ObjectBuilder.
func TestBuilderMethods[E logiface.Event](t *testing.T, cfg Config[E]) {
	t.Run(`builder methods`, func(t *testing.T) {
		t.Parallel()
		cfg.RunTest(TestRequest[E]{
			Level: logiface.LevelTrace,
		}, func(tr TestResponse[E]) {
			// 1. Slice
			tr.Logger.Info().
				Slice("slice_f", []string{"v1", "v2"}).
				Log("msg slice")

			ev, ok := tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for Slice")
			}
			if s, ok := ev.Fields["slice_f"].([]any); !ok || len(s) != 2 || s[0] != "v1" || s[1] != "v2" {
				t.Errorf("unexpected slice_f: %v", ev.Fields["slice_f"])
			}

			// 2. Map
			tr.Logger.Info().
				Map("map_f", map[string]string{"k1": "val1"}).
				Log("msg map")

			ev, ok = tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for Map")
			}
			if m, ok := ev.Fields["map_f"].(map[string]any); !ok || m["k1"] != "val1" {
				t.Errorf("unexpected map_f: %v", ev.Fields["map_f"])
			}

			// 3. MapFields
			tr.Logger.Info().
				MapFields(map[string]any{"mf_a": "alpha", "mf_b": true}).
				Log("msg mapfields")

			ev, ok = tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for MapFields")
			}
			if ev.Fields["mf_a"] != "alpha" || ev.Fields["mf_b"] != true {
				t.Errorf("unexpected mapfields: %v", ev.Fields)
			}

			// 4. ArgFields
			tr.Logger.Info().
				ArgFields[any](nil, "af_a", "beta", "af_b", true).
				Log("msg argfields")

			ev, ok = tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for ArgFields")
			}
			if ev.Fields["af_a"] != "beta" || ev.Fields["af_b"] != true {
				t.Errorf("unexpected argfields: %v", ev.Fields)
			}

			tr.SendEOFExpectNoEvents(t)
		})
	})

	t.Run(`context methods`, func(t *testing.T) {
		t.Parallel()
		cfg.RunTest(TestRequest[E]{
			Level: logiface.LevelTrace,
		}, func(tr TestResponse[E]) {
			ctxLogger := tr.Logger.Clone().
				Slice("ctx_slice", []string{"c1"}).
				Map("ctx_map", map[string]string{"ck": "cv"}).
				MapFields(map[string]any{"ctx_mf": "cmf"}).
				ArgFields[any](nil, "ctx_af", "caf").
				Logger()

			ctxLogger.Info().Log("msg context")

			ev, ok := tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for Context")
			}
			if s, ok := ev.Fields["ctx_slice"].([]any); !ok || len(s) != 1 || s[0] != "c1" {
				t.Errorf("unexpected ctx_slice: %v", ev.Fields["ctx_slice"])
			}
			if m, ok := ev.Fields["ctx_map"].(map[string]any); !ok || m["ck"] != "cv" {
				t.Errorf("unexpected ctx_map: %v", ev.Fields["ctx_map"])
			}
			if ev.Fields["ctx_mf"] != "cmf" {
				t.Errorf("unexpected ctx_mf: %v", ev.Fields["ctx_mf"])
			}
			if ev.Fields["ctx_af"] != "caf" {
				t.Errorf("unexpected ctx_af: %v", ev.Fields["ctx_af"])
			}

			tr.SendEOFExpectNoEvents(t)
		})
	})

	t.Run(`nested object and array builder methods`, func(t *testing.T) {
		t.Parallel()
		cfg.RunTest(TestRequest[E]{
			Level: logiface.LevelTrace,
		}, func(tr TestResponse[E]) {
			// ObjectBuilder with Slice, Map, MapFields, ArgFields
			tr.Logger.Info().
				Object().
				Slice("inner_slice", []string{"is1"}).
				Map("inner_map", map[string]string{"ik": "iv"}).
				MapFields(map[string]any{"inner_mf": "imf"}).
				ArgFields[any](nil, "inner_af", "iaf").
				As("parent_obj").
				End().
				Log("msg nested object")

			ev, ok := tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for nested object")
			}
			obj, ok := ev.Fields["parent_obj"].(map[string]any)
			if !ok {
				t.Fatalf("unexpected parent_obj: %v", ev.Fields["parent_obj"])
			}
			if s, ok := obj["inner_slice"].([]any); !ok || len(s) != 1 || s[0] != "is1" {
				t.Errorf("unexpected inner_slice: %v", obj["inner_slice"])
			}
			if m, ok := obj["inner_map"].(map[string]any); !ok || m["ik"] != "iv" {
				t.Errorf("unexpected inner_map: %v", obj["inner_map"])
			}
			if obj["inner_mf"] != "imf" {
				t.Errorf("unexpected inner_mf: %v", obj["inner_mf"])
			}
			if obj["inner_af"] != "iaf" {
				t.Errorf("unexpected inner_af: %v", obj["inner_af"])
			}

			// ArrayBuilder with Slice and Map
			tr.Logger.Info().
				Array().
				Slice("", []string{"as1"}).
				Map("", map[string]string{"amk": "amv"}).
				As("parent_arr").
				End().
				Log("msg nested array")

			ev, ok = tr.ReceiveEvent()
			if !ok {
				t.Fatal("expected event for nested array")
			}
			arr, ok := ev.Fields["parent_arr"].([]any)
			if !ok || len(arr) != 2 {
				t.Fatalf("unexpected parent_arr: %v", ev.Fields["parent_arr"])
			}
			if s, ok := arr[0].([]any); !ok || len(s) != 1 || s[0] != "as1" {
				t.Errorf("unexpected nested slice in array: %v", arr[0])
			}
			if m, ok := arr[1].(map[string]any); !ok || m["amk"] != "amv" {
				t.Errorf("unexpected nested map in array: %v", arr[1])
			}

			tr.SendEOFExpectNoEvents(t)
		})
	})

	t.Run(`disabled level no-op`, func(t *testing.T) {
		t.Parallel()
		cfg.RunTest(TestRequest[E]{
			Level: logiface.LevelError,
		}, func(tr TestResponse[E]) {
			// When level is disabled, methods must safely return self without panicking
			tr.Logger.Debug().
				Slice("s", []string{"a"}).
				Map("m", map[string]string{"k": "v"}).
				MapFields(map[string]any{"f": 1}).
				ArgFields[any](nil, "k", 2).
				Log("should not log")

			tr.SendEOFExpectNoEvents(t)
		})
	})
}
