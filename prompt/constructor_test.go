package prompt

import "testing"

func TestWithSyncProtocolRendererNil(t *testing.T) {
	p := &Prompt{}
	// WithSyncProtocol should not require the renderer at option-apply time.
	if err := WithSyncProtocol(true)(p); err != nil {
		t.Errorf("WithSyncProtocol should not error when renderer is nil: %v", err)
	}
	if !p.syncEnabled {
		t.Error("WithSyncProtocol should render with syncEnabled")
	}
}
