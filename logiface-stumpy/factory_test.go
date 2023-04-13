package stumpy

import (
	"bytes"
	"errors"
	"testing"
)

func TestWithMessageField(t *testing.T) {
	var b bytes.Buffer
	L.New(L.WithStumpy(WithWriter(&b), WithLevelField(``), WithMessageField(`foo`))).Crit().Log(`bar`)
	if s := b.String(); s != `{"foo":"bar"}`+"\n" {
		t.Error(s)
	}
}

func TestWithErrorField(t *testing.T) {
	var b bytes.Buffer
	L.New(L.WithStumpy(WithWriter(&b), WithLevelField(``), WithErrorField(`foo`))).Crit().Err(errors.New(`bar`)).Log(``)
	if s := b.String(); s != `{"foo":"bar"}`+"\n" {
		t.Error(s)
	}
}
