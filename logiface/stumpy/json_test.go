package stumpy

import (
	"github.com/joeycumines/go-utilpkg/logiface"
	"os"
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
