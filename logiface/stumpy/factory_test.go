package stumpy

import (
	"bytes"
	"testing"
)

func TestWithMessageField(t *testing.T) {
	var b bytes.Buffer
	L.New(L.WithStumpy(WithWriter(&b), WithLevelField(``), WithMessageField(`foo`))).Crit().Log(`bar`)
	if s := b.String(); s != `{"foo":"bar"}`+"\n" {
		t.Error(s)
	}
}
