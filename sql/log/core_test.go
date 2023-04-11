package log

import (
	"testing"
)

func TestDiscard_WithField(t *testing.T) {
	if (Discard{}).WithField(``, nil) != (Discard{}) {
		t.Error()
	}
}

func TestDiscard_WithFields(t *testing.T) {
	if (Discard{}).WithFields(nil) != (Discard{}) {
		t.Error()
	}
}

func TestDiscard_WithError(t *testing.T) {
	if (Discard{}).WithError(nil) != (Discard{}) {
		t.Error()
	}
}

func TestDiscard_Debug(t *testing.T) {
	(Discard{}).Debug()
}

func TestDiscard_Info(t *testing.T) {
	(Discard{}).Info()
}

func TestDiscard_Warn(t *testing.T) {
	(Discard{}).Warn()
}

func TestDiscard_Error(t *testing.T) {
	(Discard{}).Error()
}
