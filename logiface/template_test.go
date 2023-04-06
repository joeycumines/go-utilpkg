package logiface_test

import (
	"bytes"
	"fmt"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/joeycumines/go-utilpkg/logiface/internal/mocklog"
	"github.com/joeycumines/go-utilpkg/logiface/internal/stumpy"
	"io"
	"strings"
	"testing"
)

type (
	eventTemplate struct {
		Stumpy                  func(t *testing.T, s string)
		Mocklog                 func(t *testing.T, s string)
		Fluent                  func(logger *logiface.Logger[logiface.Event])
		CallForNesting          func(logger *logiface.Logger[logiface.Event])
		CallForNestingSansChain func(logger *logiface.Logger[logiface.Event])
	}
)

var (
	eventTemplates = [...]*eventTemplate{
		newEventTemplate1(),
		newEventTemplate2(),
		newEventTemplate3(),
		newEventTemplate4(),
		newEventTemplate5(),
		newEventTemplate6(),
	}
)

func newEventTemplateStumpyLogger(w io.Writer, enabled bool) *logiface.Logger[logiface.Event] {
	lvl := stumpy.L.WithLevel(logiface.LevelTrace)
	if !enabled {
		lvl = stumpy.L.WithLevel(logiface.LevelError)
	}
	return stumpy.L.New(stumpy.L.WithStumpy(stumpy.WithWriter(w)), lvl).Logger()
}

func newEventTemplateMocklogLogger(w io.Writer, enabled bool) *logiface.Logger[logiface.Event] {
	lvl := mocklog.L.WithLevel(logiface.LevelTrace)
	if !enabled {
		lvl = mocklog.L.WithLevel(logiface.LevelError)
	}
	return mocklog.L.New(mocklog.WithMocklog(&mocklog.Writer{
		Writer: w,
		JSON:   true,
	}), lvl).Logger()
}

func TestEventTemplate(t *testing.T) {
	for i, template := range eventTemplates {
		template := template
		t.Run(fmt.Sprintf(`template%d`, i+1), func(t *testing.T) {
			for _, tc1 := range [...]struct {
				Name      string
				Enabled   bool
				NilLogger bool
			}{
				{
					Name:      `enabled`,
					Enabled:   true,
					NilLogger: false,
				},
				{
					Name:      `disabled`,
					Enabled:   false,
					NilLogger: false,
				},
				{
					Name:      `nilLogger`,
					Enabled:   false,
					NilLogger: true,
				},
			} {
				tc1 := tc1
				t.Run(tc1.Name, func(t *testing.T) {
					for _, tc2 := range [...]struct {
						Name    string
						Factory func(w io.Writer, enabled bool) *logiface.Logger[logiface.Event]
						Assert  func(t *testing.T, s string)
					}{
						{
							Name:    `stumpy`,
							Factory: newEventTemplateStumpyLogger,
							Assert:  template.Stumpy,
						},
						{
							Name:    `mocklog`,
							Factory: newEventTemplateMocklogLogger,
							Assert:  template.Mocklog,
						},
					} {
						tc2 := tc2
						t.Run(tc2.Name, func(t *testing.T) {
							if tc2.Assert == nil {
								t.Skip(`not implemented`)
							}
							for _, tc3 := range [...]struct {
								Name string
								Log  func(logger *logiface.Logger[logiface.Event])
							}{
								{
									Name: `Fluent`,
									Log:  template.Fluent,
								},
								{
									Name: `CallForNesting`,
									Log:  template.CallForNesting,
								},
								{
									Name: `CallForNestingSansChain`,
									Log:  template.CallForNestingSansChain,
								},
							} {
								tc3 := tc3
								t.Run(tc3.Name, func(t *testing.T) {
									if tc3.Log == nil {
										t.Skip(`not implemented`)
									}
									var buffer bytes.Buffer
									var logger *logiface.Logger[logiface.Event]
									if !tc1.NilLogger {
										logger = tc2.Factory(&buffer, tc1.Enabled)
									}
									tc3.Log(logger)
									if s := buffer.String(); tc1.Enabled {
										tc2.Assert(t, s)
										t.Log(strings.TrimSpace(s))
									} else if s != `` {
										t.Errorf("expected no output, got %q\n%s", s, s)
									} else {
										t.Log(`no output`)
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

func newEventTemplate1() *eventTemplate {
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

	var t eventTemplate

	t.Stumpy = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"lvl":"info","request_id":"c7d5a8f1-7e39-4d07-a9f5-73b96d31c036","user_id":1234,"username":"johndoe","roles":[{"id":1,"name":"admin"},{"id":2,"name":"user"}],"preferences":{"language":"en","notifications":{"email":true,"sms":false}},"endpoint":"/api/v1/users","method":"GET","response":{"status":200,"users":[{"id":5678,"username":"janedoe","email":"janedoe@example.com","groups":[{"id":101,"name":"group1"},{"id":102,"name":"group2"}]},{"id":9101,"username":"mike92","email":"mike92@example.com","groups":[{"id":103,"name":"group3"}]}]},"elapsed":230,"unit":"ms","msg":"API request processed"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Mocklog = func(t *testing.T, s string) {
		t.Helper()
		if s != `[info] request_id="c7d5a8f1-7e39-4d07-a9f5-73b96d31c036" user_id=1234 username="johndoe" roles=[{"id":1,"name":"admin"},{"id":2,"name":"user"}] preferences={"language":"en","notifications":{"email":true,"sms":false}} endpoint="/api/v1/users" method="GET" response={"status":200,"users":[{"email":"janedoe@example.com","groups":[{"id":101,"name":"group1"},{"id":102,"name":"group2"}],"id":5678,"username":"janedoe"},{"email":"mike92@example.com","groups":[{"id":103,"name":"group3"}],"id":9101,"username":"mike92"}]} elapsed=230 unit="ms" msg="API request processed"`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Fluent = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Str("request_id", requestID).
			Int("user_id", userID).
			Str("username", username).
			//>roles[*]
			Array().
			//>roles[0].*
			Object().
			Int("id", role1ID).
			Str("name", role1Name).
			Add().
			//<roles[0].*
			//>roles[1].*
			Object().
			Int("id", role2ID).
			Str("name", role2Name).
			Add().
			//<roles[1].*
			As("roles").
			//<roles[*]
			//>preferences.*
			Object().
			Str("language", language).
			//>preferences.notifications.*
			Object().
			Bool("email", emailNotification).
			Bool("sms", smsNotification).
			As("notifications").
			//<preferences.notifications.*
			As("preferences").
			//<preferences.*
			End().
			Str("endpoint", endpoint).
			Str("method", method).
			//>response.*
			Object().
			Int("status", responseStatus).
			//>response.users[*]
			Array().
			//>response.users[0].*
			Object().
			Int("id", user1ID).
			Str("username", user1Username).
			Str("email", user1Email).
			//>response.users[0].groups[*]
			Array().
			//>response.users[0].groups[0].*
			Object().
			Int("id", group1ID).
			Str("name", group1Name).
			Add().
			//<response.users[0].groups[0].*
			//>response.users[0].groups[1].*
			Object().
			Int("id", group2ID).
			Str("name", group2Name).
			Add().
			//<response.users[0].groups[1].*
			As("groups").
			//<response.users[0].groups[*]
			Add().
			//<response.users[0].*
			//>response.users[1].*
			Object().
			Int("id", user2ID).
			Str("username", user2Username).
			Str("email", user2Email).
			//>response.users[1].groups[*]
			Array().
			//>response.users[1].groups[0].*
			Object().
			Int("id", group3ID).
			Str("name", group3Name).
			Add().
			//<response.users[1].groups[0].*
			As("groups").
			//<response.users[1].groups[*]
			Add().
			//<response.users[1].*
			As("users").
			//<response.users[*]
			As("response").
			//<response.*
			End().
			Int("elapsed", elapsed).
			Str("unit", unit).
			Log("API request processed")
	}

	t.CallForNesting = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Str("request_id", requestID).
			Int("user_id", userID).
			Str("username", username).
			Call(func(b *logiface.Builder[logiface.Event]) {
				b.Array().
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
						b.Object().
							Int("id", role1ID).
							Str("name", role1Name).
							Add().
							End()
					}).
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
						b.Object().
							Int("id", role2ID).
							Str("name", role2Name).
							Add().
							End()
					}).
					As("roles").
					End()
			}).
			Call(func(b *logiface.Builder[logiface.Event]) {
				b.Object().
					Str("language", language).
					Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
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
			Call(func(b *logiface.Builder[logiface.Event]) {
				b.Object().
					Int("status", responseStatus).
					Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
						b.Array().
							Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
								b.Object().
									Int("id", user1ID).
									Str("username", user1Username).
									Str("email", user1Email).
									Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
										b.Array().
											Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
												b.Object().
													Int("id", group1ID).
													Str("name", group1Name).
													Add().
													End()
											}).
											Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
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
							Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
								b.Object().
									Int("id", user2ID).
									Str("username", user2Username).
									Str("email", user2Email).
									Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
										b.Array().
											Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
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
	}

	t.CallForNestingSansChain = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Str("request_id", requestID).
			Int("user_id", userID).
			Str("username", username).
			Call(func(b *logiface.Builder[logiface.Event]) {
				logiface.Array[logiface.Event](b).
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Builder[logiface.Event]]) {
						logiface.Object[logiface.Event](b).
							Int("id", role1ID).
							Str("name", role1Name).
							Add()
					}).
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Builder[logiface.Event]]) {
						logiface.Object[logiface.Event](b).
							Int("id", role2ID).
							Str("name", role2Name).
							Add()
					}).
					As("roles")
			}).
			Call(func(b *logiface.Builder[logiface.Event]) {
				logiface.Object[logiface.Event](b).
					Str("language", language).
					Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]) {
						logiface.Object[logiface.Event](b).
							Bool("email", emailNotification).
							Bool("sms", smsNotification).
							As("notifications")
					}).
					As("preferences")
			}).
			Str("endpoint", endpoint).
			Str("method", method).
			Call(func(b *logiface.Builder[logiface.Event]) {
				logiface.Object[logiface.Event](b).
					Int("status", responseStatus).
					Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]) {
						logiface.Array[logiface.Event](b).
							Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]) {
								logiface.Object[logiface.Event](b).
									Int("id", user1ID).
									Str("username", user1Username).
									Str("email", user1Email).
									Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]]) {
										logiface.Array[logiface.Event](b).
											Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]]]) {
												logiface.Object[logiface.Event](b).
													Int("id", group1ID).
													Str("name", group1Name).
													Add()
											}).
											Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]]]) {
												logiface.Object[logiface.Event](b).
													Int("id", group2ID).
													Str("name", group2Name).
													Add()
											}).
											As("groups")
									}).
									Add()
							}).
							Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]) {
								logiface.Object[logiface.Event](b).
									Int("id", user2ID).
									Str("username", user2Username).
									Str("email", user2Email).
									Call(func(b *logiface.ObjectBuilder[logiface.Event, *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]]) {
										logiface.Array[logiface.Event](b).
											Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.ArrayBuilder[logiface.Event, *logiface.ObjectBuilder[logiface.Event, *logiface.Builder[logiface.Event]]]]]) {
												logiface.Object[logiface.Event](b).
													Int("id", group3ID).
													Str("name", group3Name).
													Add()
											}).
											As("groups")
									}).
									Add()
							}).
							As("users")
					}).
					As("response")
			}).
			Int("elapsed", elapsed).
			Str("unit", unit).
			Log("API request processed")
	}

	return &t
}

func newEventTemplate2() *eventTemplate {
	requestID := "3c3cb9d9-9a14-41e2-a23a-49d50f684fbf"
	userID := 123
	username := "alice"
	age := 27
	country := "USA"
	state := "CA"

	var t eventTemplate

	t.Stumpy = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"lvl":"info","request_id":"3c3cb9d9-9a14-41e2-a23a-49d50f684fbf","user_id":123,"username":"alice","age":27,"location":{"country":"USA","state":"CA"},"msg":"User information updated"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Mocklog = func(t *testing.T, s string) {
		t.Helper()
		if s != `[info] request_id="3c3cb9d9-9a14-41e2-a23a-49d50f684fbf" user_id=123 username="alice" age=27 location={"country":"USA","state":"CA"} msg="User information updated"`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Fluent = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Str("request_id", requestID).
			Int("user_id", userID).
			Str("username", username).
			Int("age", age).
			Object().
			Str("country", country).
			Str("state", state).
			As("location").
			End().
			Log("User information updated")
	}

	return &t
}

func newEventTemplate3() *eventTemplate {
	serverID := "webserver-01"
	serverIP := "192.168.0.10"
	serverHostname := "webserver-01.example.com"
	osType := "Linux"
	osVersion := "5.4.0-91-generic"
	loadAverage1 := 0.25
	loadAverage5 := 0.17
	loadAverage15 := 0.12
	processes := 130

	app1Name := "webapp-01"
	app1Port := 8080
	app1Status := "running"
	app2Name := "webapp-02"
	app2Port := 8081
	app2Status := "stopped"

	var t eventTemplate

	t.Stumpy = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"lvl":"info","server_id":"webserver-01","server_ip":"192.168.0.10","server_hostname":"webserver-01.example.com","os":{"type":"Linux","version":"5.4.0-91-generic"},"load_average":[0.25,0.17,0.12],"processes":130,"apps":[{"name":"webapp-01","port":8080,"status":"running"},{"name":"webapp-02","port":8081,"status":"stopped"}],"msg":"Server status updated"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Mocklog = func(t *testing.T, s string) {
		t.Helper()
		if s != `[info] server_id="webserver-01" server_ip="192.168.0.10" server_hostname="webserver-01.example.com" os={"type":"Linux","version":"5.4.0-91-generic"} load_average=[0.25,0.17,0.12] processes=130 apps=[{"name":"webapp-01","port":8080,"status":"running"},{"name":"webapp-02","port":8081,"status":"stopped"}] msg="Server status updated"`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Fluent = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Str("server_id", serverID).
			Str("server_ip", serverIP).
			Str("server_hostname", serverHostname).
			//>os.*
			Object().
			Str("type", osType).
			Str("version", osVersion).
			As(`os`).
			//<os.*
			//>load_average[*]
			Array().
			Float64(loadAverage1).
			Float64(loadAverage5).
			Float64(loadAverage15).
			As("load_average").
			//<load_average[*]
			End().
			Int("processes", processes).
			//>apps[*]
			Array().
			//>apps[0].*
			Object().
			Str("name", app1Name).
			Int("port", app1Port).
			Str("status", app1Status).
			Add().
			//<apps[0].*
			//>apps[1].*
			Object().
			Str("name", app2Name).
			Int("port", app2Port).
			Str("status", app2Status).
			Add().
			//<apps[1].*
			As("apps").
			//<apps[*]
			End().
			Log("Server status updated")
	}

	return &t
}

func newEventTemplate4() *eventTemplate {
	orderID := "ORD-12345"
	customerName := "John Smith"
	customerID := 5678
	deliveryMethod := "Standard"
	orderTotal := 124.99
	currency := "USD"
	orderDate := "2023-03-25"
	estimatedDeliveryDate := "2023-03-31"
	trackingNumber := "1234567890"

	productID := "P123"
	productName := "Widget"
	productPrice := 24.99
	productQuantity := 5

	var t eventTemplate

	t.Stumpy = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"lvl":"info","order_id":"ORD-12345","customer":{"name":"John Smith","id":5678},"delivery_method":"Standard","order_total":124.99,"currency":"USD","order_date":"2023-03-25","estimated_delivery_date":"2023-03-31","tracking_number":"1234567890","items":[{"product_id":"P123","name":"Widget","price":24.99,"quantity":5}],"msg":"Order placed"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Mocklog = func(t *testing.T, s string) {
		t.Helper()
		if s != `[info] order_id="ORD-12345" customer={"id":5678,"name":"John Smith"} delivery_method="Standard" order_total=124.99 currency="USD" order_date="2023-03-25" estimated_delivery_date="2023-03-31" tracking_number="1234567890" items=[{"name":"Widget","price":24.99,"product_id":"P123","quantity":5}] msg="Order placed"`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Fluent = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Str("order_id", orderID).
			Object().
			Str("name", customerName).
			Int("id", customerID).
			As("customer").
			End().
			Str("delivery_method", deliveryMethod).
			Float64("order_total", orderTotal).
			Str("currency", currency).
			Str("order_date", orderDate).
			Str("estimated_delivery_date", estimatedDeliveryDate).
			Str("tracking_number", trackingNumber).
			Array().
			Object().
			Str("product_id", productID).
			Str("name", productName).
			Float64("price", productPrice).
			Int("quantity", productQuantity).
			Add().
			As("items").
			End().
			Log("Order placed")
	}

	return &t
}

func newEventTemplate5() *eventTemplate {
	var t eventTemplate

	t.Stumpy = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"lvl":"info","k":[{}]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Mocklog = func(t *testing.T, s string) {
		t.Helper()
		if s != `[info] k=[{}]`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Fluent = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Array().
			Object().
			Add().
			As("k").
			End().
			Log("")
	}

	t.CallForNesting = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Call(func(b *logiface.Builder[logiface.Event]) {
				b.Array().
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Builder[logiface.Event]]]) {
						b.Object().
							Add().
							End()
					}).
					As("k").
					End()
			}).
			Log("")
	}

	t.CallForNestingSansChain = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Call(func(b *logiface.Builder[logiface.Event]) {
				logiface.Array[logiface.Event](b).
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Builder[logiface.Event]]) {
						logiface.Object[logiface.Event](b).
							Add()
					}).
					As("k")
			}).
			Log("")
	}

	return &t
}

// newEventTemplate6 is a copy of newEventTemplate5 that's built using context
func newEventTemplate6() *eventTemplate {
	var t eventTemplate

	t.Stumpy = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"lvl":"info","k":[{}]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Mocklog = func(t *testing.T, s string) {
		t.Helper()
		if s != `[info] k=[{}]`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Fluent = func(logger *logiface.Logger[logiface.Event]) {
		logger.Clone().
			Array().
			Object().
			Add().
			As("k").
			End().
			Logger().
			Info().
			Log("")
	}

	t.CallForNesting = func(logger *logiface.Logger[logiface.Event]) {
		logger.Clone().
			Call(func(b *logiface.Context[logiface.Event]) {
				b.Array().
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Chain[logiface.Event, *logiface.Context[logiface.Event]]]) {
						b.Object().
							Add().
							End()
					}).
					As("k").
					End()
			}).
			Logger().
			Info().
			Log("")
	}

	t.CallForNestingSansChain = func(logger *logiface.Logger[logiface.Event]) {
		logger.Clone().
			Call(func(b *logiface.Context[logiface.Event]) {
				logiface.Array[logiface.Event](b).
					Call(func(b *logiface.ArrayBuilder[logiface.Event, *logiface.Context[logiface.Event]]) {
						logiface.Object[logiface.Event](b).
							Add()
					}).
					As("k")
			}).
			Logger().
			Info().
			Log("")
	}

	return &t
}
