package zerolog

import (
	"bytes"
	"fmt"
	"github.com/joeycumines/go-utilpkg/logiface"
	"github.com/rs/zerolog"
	"io"
	"testing"
)

type (
	eventTemplate struct {
		Assert    func(t *testing.T, s string)
		Baseline  func(logger *zerolog.Logger)
		Generic   func(logger *logiface.Logger[*Event])
		Interface func(logger *logiface.Logger[logiface.Event])
	}
)

var (
	eventTemplates = [...]*eventTemplate{
		newEventTemplate1(),
		newEventTemplate2(),
		newEventTemplate3(),
		newEventTemplate4(),
		newEventTemplate5(),
	}
)

func newEventTemplateBaselineLogger(w io.Writer) (l *zerolog.Logger) {
	l = new(zerolog.Logger)
	*l = zerolog.New(w)
	return
}

func newEventTemplateGenericLogger(w io.Writer) *logiface.Logger[*Event] {
	return L.New(L.WithZerolog(*newEventTemplateBaselineLogger(w)))
}

func newEventTemplateInterfaceLogger(w io.Writer) *logiface.Logger[logiface.Event] {
	return L.New(L.WithZerolog(*newEventTemplateBaselineLogger(w))).Logger()
}

func TestEventTemplate_Baseline(t *testing.T) {
	t.Parallel()
	for i, template := range eventTemplates {
		template := template
		t.Run(fmt.Sprintf(`template%d`, i+1), func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			logger := newEventTemplateBaselineLogger(&buffer)
			template.Baseline(logger)
			template.Assert(t, buffer.String())
		})
	}
}

func TestEventTemplate_Generic(t *testing.T) {
	t.Parallel()
	for i, template := range eventTemplates {
		template := template
		t.Run(fmt.Sprintf(`template%d`, i+1), func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			logger := newEventTemplateGenericLogger(&buffer)
			template.Generic(logger)
			template.Assert(t, buffer.String())
		})
	}
}

func TestEventTemplate_Interface(t *testing.T) {
	t.Parallel()
	for i, template := range eventTemplates {
		template := template
		t.Run(fmt.Sprintf(`template%d`, i+1), func(t *testing.T) {
			t.Parallel()
			var buffer bytes.Buffer
			logger := newEventTemplateInterfaceLogger(&buffer)
			template.Interface(logger)
			template.Assert(t, buffer.String())
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

	t.Assert = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"level":"info","request_id":"c7d5a8f1-7e39-4d07-a9f5-73b96d31c036","user_id":1234,"username":"johndoe","roles":[{"id":1,"name":"admin"},{"id":2,"name":"user"}],"preferences":{"language":"en","notifications":{"email":true,"sms":false}},"endpoint":"/api/v1/users","method":"GET","response":{"status":200,"users":[{"id":5678,"username":"janedoe","email":"janedoe@example.com","groups":[{"id":101,"name":"group1"},{"id":102,"name":"group2"}]},{"id":9101,"username":"mike92","email":"mike92@example.com","groups":[{"id":103,"name":"group3"}]}]},"elapsed":230,"unit":"ms","message":"API request processed"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Baseline = func(logger *zerolog.Logger) {
		logger.Info().
			Str("request_id", requestID).
			Int("user_id", userID).
			Str("username", username).
			Array("roles", zerolog.Arr().
				Dict(zerolog.Dict().
					Int("id", role1ID).
					Str("name", role1Name)).
				Dict(zerolog.Dict().
					Int("id", role2ID).
					Str("name", role2Name))).
			Dict("preferences", zerolog.Dict().
				Str("language", language).
				Dict("notifications", zerolog.Dict().
					Bool("email", emailNotification).
					Bool("sms", smsNotification))).
			Str("endpoint", endpoint).
			Str("method", method).
			Dict("response", zerolog.Dict().
				Int("status", responseStatus).
				Array("users", zerolog.Arr().
					Dict(zerolog.Dict().
						Int("id", user1ID).
						Str("username", user1Username).
						Str("email", user1Email).
						Array("groups", zerolog.Arr().
							Dict(zerolog.Dict().
								Int("id", group1ID).
								Str("name", group1Name)).
							Dict(zerolog.Dict().
								Int("id", group2ID).
								Str("name", group2Name)))).
					Dict(zerolog.Dict().
						Int("id", user2ID).
						Str("username", user2Username).
						Str("email", user2Email).
						Array("groups", zerolog.Arr().Dict(zerolog.Dict().
							Int("id", group3ID).
							Str("name", group3Name)))))).
			Int("elapsed", elapsed).
			Str("unit", unit).
			Msg("API request processed")
	}

	t.Generic = func(logger *logiface.Logger[*Event]) {
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

	t.Interface = func(logger *logiface.Logger[logiface.Event]) {
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

	t.Assert = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"level":"info","request_id":"3c3cb9d9-9a14-41e2-a23a-49d50f684fbf","user_id":123,"username":"alice","age":27,"location":{"country":"USA","state":"CA"},"message":"User information updated"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Baseline = func(logger *zerolog.Logger) {
		logger.Info().
			Str("request_id", requestID).
			Int("user_id", userID).
			Str("username", username).
			Int("age", age).
			Dict("location", zerolog.Dict().
				Str("country", country).
				Str("state", state)).
			Msg("User information updated")
	}

	t.Generic = func(logger *logiface.Logger[*Event]) {
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

	t.Interface = func(logger *logiface.Logger[logiface.Event]) {
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

	t.Assert = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"level":"info","server_id":"webserver-01","server_ip":"192.168.0.10","server_hostname":"webserver-01.example.com","os":{"type":"Linux","version":"5.4.0-91-generic"},"load_average":[0.25,0.17,0.12],"processes":130,"apps":[{"name":"webapp-01","port":8080,"status":"running"},{"name":"webapp-02","port":8081,"status":"stopped"}],"message":"Server status updated"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Baseline = func(logger *zerolog.Logger) {
		logger.Info().
			Str("server_id", serverID).
			Str("server_ip", serverIP).
			Str("server_hostname", serverHostname).
			Dict("os", zerolog.Dict().
				Str("type", osType).
				Str("version", osVersion)).
			Array("load_average", zerolog.Arr().
				Float64(loadAverage1).
				Float64(loadAverage5).
				Float64(loadAverage15)).
			Int("processes", processes).
			Array("apps", zerolog.Arr().
				Dict(zerolog.Dict().
					Str("name", app1Name).
					Int("port", app1Port).
					Str("status", app1Status)).
				Dict(zerolog.Dict().
					Str("name", app2Name).
					Int("port", app2Port).
					Str("status", app2Status))).
			Msg("Server status updated")
	}

	t.Generic = func(logger *logiface.Logger[*Event]) {
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

	t.Interface = func(logger *logiface.Logger[logiface.Event]) {
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

	t.Assert = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"level":"info","order_id":"ORD-12345","customer":{"name":"John Smith","id":5678},"delivery_method":"Standard","order_total":124.99,"currency":"USD","order_date":"2023-03-25","estimated_delivery_date":"2023-03-31","tracking_number":"1234567890","items":[{"product_id":"P123","name":"Widget","price":24.99,"quantity":5}],"message":"Order placed"}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Baseline = func(logger *zerolog.Logger) {
		logger.Info().
			Str("order_id", orderID).
			Dict("customer", zerolog.Dict().
				Str("name", customerName).
				Int("id", customerID)).
			Str("delivery_method", deliveryMethod).
			Float64("order_total", orderTotal).
			Str("currency", currency).
			Str("order_date", orderDate).
			Str("estimated_delivery_date", estimatedDeliveryDate).
			Str("tracking_number", trackingNumber).
			Array("items", zerolog.Arr().
				Dict(zerolog.Dict().
					Str("product_id", productID).
					Str("name", productName).
					Float64("price", productPrice).
					Int("quantity", productQuantity))).
			Msg("Order placed")
	}

	t.Generic = func(logger *logiface.Logger[*Event]) {
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

	t.Interface = func(logger *logiface.Logger[logiface.Event]) {
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

	t.Assert = func(t *testing.T, s string) {
		t.Helper()
		if s != `{"level":"info","k":[{}]}`+"\n" {
			t.Errorf("unexpected output: %q\n%s", s, s)
		}
	}

	t.Baseline = func(logger *zerolog.Logger) {
		logger.Info().
			Array("k", zerolog.Arr().
				Dict(zerolog.Dict())).
			Send()
	}

	t.Generic = func(logger *logiface.Logger[*Event]) {
		logger.Info().
			Array().
			Object().
			Add().
			As("k").
			End().
			Log("")
	}

	t.Interface = func(logger *logiface.Logger[logiface.Event]) {
		logger.Info().
			Array().
			Object().
			Add().
			As("k").
			End().
			Log("")
	}

	return &t
}
