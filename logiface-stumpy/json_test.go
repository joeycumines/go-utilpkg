package stumpy

import (
	"bytes"
	"encoding/json"
	"github.com/joeycumines/logiface"
	"os"
	"testing"
	"time"
)

func ExampleLogger_json1() {
	{
		old := timeNow
		defer func() { timeNow = old }()
		timeNow = func() time.Time {
			return time.Unix(1680693679, 496235772)
		}
	}

	logger := L.New(L.WithStumpy(
		WithWriter(os.Stdout),
		WithTimeField(`timestamp`),
		WithLevelField(`level`),
	))

	requestID := "c7d5a8f1-7e39-4d07-a9f5-73b96d31c036"
	userID := 1234
	username := "johndoe"
	role1ID := 1
	role1Name := "admin"
	role2ID := 2
	role2Name := "user"
	language := "en"
	emailNotification := true
	smsNotification := false
	endpoint := "/api/v1/users"
	method := "GET"
	responseStatus := 200
	elapsed := 230
	unit := "ms"

	user1ID := 5678
	user1Username := "janedoe"
	user1Email := "janedoe@example.com"
	group1ID := 101
	group1Name := "group1"
	group2ID := 102
	group2Name := "group2"

	user2ID := 9101
	user2Username := "mike92"
	user2Email := "mike92@example.com"
	group3ID := 103
	group3Name := "group3"

	logger.Info().
		Str("request_id", requestID).
		Int("user_id", userID).
		Str("username", username).
		Call(func(b *logiface.Builder[*Event]) {
			b.Array().
				Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
					b.Object().
						Int("id", role1ID).
						Str("name", role1Name).
						Add().
						End()
				}).
				Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
					b.Object().
						Int("id", role2ID).
						Str("name", role2Name).
						Add().
						End()
				}).
				As("roles").
				End()
		}).
		Call(func(b *logiface.Builder[*Event]) {
			b.Object().
				Str("language", language).
				Call(func(b *logiface.ObjectBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
					b.Object().
						Bool("email", emailNotification).
						Bool("sms", smsNotification).
						As("notifications").
						End()
				}).
				As("preferences").
				End()
		}).
		Str("endpoint", endpoint).
		Str("method", method).
		Call(func(b *logiface.Builder[*Event]) {
			b.Object().
				Int("status", responseStatus).
				Call(func(b *logiface.ObjectBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
					b.Array().
						Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
							b.Object().
								Int("id", user1ID).
								Str("username", user1Username).
								Str("email", user1Email).
								Call(func(b *logiface.ObjectBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
									b.Array().
										Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
											b.Object().
												Int("id", group1ID).
												Str("name", group1Name).
												Add().
												End()
										}).
										Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
											b.Object().
												Int("id", group2ID).
												Str("name", group2Name).
												Add().
												End()
										}).
										As("groups").
										End()
								}).
								Add().
								End()
						}).
						Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
							b.Object().
								Int("id", user2ID).
								Str("username", user2Username).
								Str("email", user2Email).
								Call(func(b *logiface.ObjectBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
									b.Array().
										Call(func(b *logiface.ArrayBuilder[*Event, *logiface.Chain[*Event, *logiface.Builder[*Event]]]) {
											b.Object().
												Int("id", group3ID).
												Str("name", group3Name).
												Add().
												End()
										}).
										As("groups").
										End()
								}).
								Add().
								End()
						}).
						As("users").
						End()
				}).
				As("response").
				End()
		}).
		Int("elapsed", elapsed).
		Str("unit", unit).
		Log("API request processed")

	//output:
	//{"timestamp":"2023-04-05T11:21:19.496235772Z","level":"info","request_id":"c7d5a8f1-7e39-4d07-a9f5-73b96d31c036","user_id":1234,"username":"johndoe","roles":[{"id":1,"name":"admin"},{"id":2,"name":"user"}],"preferences":{"language":"en","notifications":{"email":true,"sms":false}},"endpoint":"/api/v1/users","method":"GET","response":{"status":200,"users":[{"id":5678,"username":"janedoe","email":"janedoe@example.com","groups":[{"id":101,"name":"group1"},{"id":102,"name":"group2"}]},{"id":9101,"username":"mike92","email":"mike92@example.com","groups":[{"id":103,"name":"group3"}]}]},"elapsed":230,"unit":"ms","msg":"API request processed"}
}

func TestLogger_builderMethods(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := L.New(L.WithStumpy(
		WithWriter(&buf),
		WithLevelField(`level`),
		WithMessageField(`msg`),
	))

	t.Run(`builder methods`, func(t *testing.T) {
		buf.Reset()
		logger.Info().
			Slice("slice_f", []string{"a", "b"}).
			Map("map_f", map[string]string{"k1": "v1"}).
			MapFields(map[string]any{"mf_a": "alpha"}).
			ArgFields[any](nil, "af_a", "beta").
			Log("test builder methods")

		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if m["msg"] != "test builder methods" {
			t.Errorf("unexpected msg: %v", m["msg"])
		}
		if s, ok := m["slice_f"].([]any); !ok || len(s) != 2 || s[0] != "a" || s[1] != "b" {
			t.Errorf("unexpected slice_f: %v", m["slice_f"])
		}
		if mf, ok := m["map_f"].(map[string]any); !ok || mf["k1"] != "v1" {
			t.Errorf("unexpected map_f: %v", m["map_f"])
		}
		if m["mf_a"] != "alpha" {
			t.Errorf("unexpected mf_a: %v", m["mf_a"])
		}
		if m["af_a"] != "beta" {
			t.Errorf("unexpected af_a: %v", m["af_a"])
		}
	})

	t.Run(`context builder methods`, func(t *testing.T) {
		buf.Reset()
		ctxLogger := logger.Clone().
			Slice("ctx_slice", []string{"c1"}).
			Map("ctx_map", map[string]string{"ck": "cv"}).
			MapFields(map[string]any{"ctx_mf": "cmf"}).
			ArgFields[any](nil, "ctx_af", "caf").
			Logger()

		ctxLogger.Info().Log("test context builder methods")

		var m map[string]any
		if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
			t.Fatalf("failed to decode JSON: %v", err)
		}
		if m["msg"] != "test context builder methods" {
			t.Errorf("unexpected msg: %v", m["msg"])
		}
		if s, ok := m["ctx_slice"].([]any); !ok || len(s) != 1 || s[0] != "c1" {
			t.Errorf("unexpected ctx_slice: %v", m["ctx_slice"])
		}
		if mf, ok := m["ctx_map"].(map[string]any); !ok || mf["ck"] != "cv" {
			t.Errorf("unexpected ctx_map: %v", m["ctx_map"])
		}
		if m["ctx_mf"] != "cmf" {
			t.Errorf("unexpected ctx_mf: %v", m["ctx_mf"])
		}
		if m["ctx_af"] != "caf" {
			t.Errorf("unexpected ctx_af: %v", m["ctx_af"])
		}
	})
}
