package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// Intl API Verification Tests
// Tests verify Goja's support for Intl API:
// - Intl object existence
// - Intl.Collator (string comparison)
// - Intl.DateTimeFormat (date formatting)
// - Intl.NumberFormat (number formatting)
// - Intl.PluralRules (pluralization)
// - Intl.RelativeTimeFormat (relative time)
// - Intl.ListFormat (list formatting)
// - Intl.Segmenter (text segmentation)
// - Intl.DisplayNames (display name translation)
// - Intl.getCanonicalLocales()
// - Intl.supportedValuesOf()
//
// NOTE: Goja may not implement full Intl support.
// Tests check availability gracefully and skip if not present.
// ===============================================

func newIntlTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("Bind failed: %v", err)
	}

	cleanup := func() {
		loop.Shutdown(context.Background())
	}

	return adapter, runtime, cleanup
}

// ===============================================
// 1. Intl Object Existence
// ===============================================

func TestIntl_ObjectExists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.String() == "object" {
		t.Log("Intl: NATIVE (object exists)")
	} else if v.String() == "undefined" {
		t.Log("Intl: NOT_AVAILABLE (Goja does not implement Intl)")
		t.Skip("Intl not available in Goja")
	} else {
		t.Errorf("unexpected typeof Intl: %s", v.String())
	}
}

func TestIntl_ObjectProperties(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	// Check if Intl exists first
	vIntl, err := runtime.RunString(`typeof Intl`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if vIntl.String() == "undefined" {
		t.Skip("Intl not available in Goja")
	}

	// If Intl exists, check what properties it has
	script := `
		var props = [];
		for (var k in Intl) {
			props.push(k);
		}
		Object.getOwnPropertyNames(Intl).forEach(function(p) {
			if (props.indexOf(p) === -1) props.push(p);
		});
		JSON.stringify(props.sort());
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	t.Logf("Intl properties: %s", v.String())
}

// ===============================================
// 2. Intl.Collator (string comparison)
// ===============================================

func TestIntl_Collator_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.Collator === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.Collator: NATIVE")
	} else {
		t.Log("Intl.Collator: NOT_AVAILABLE")
		t.Skip("Intl.Collator not available")
	}
}

func TestIntl_Collator_Compare(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	// Check availability
	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.Collator === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.Collator not available")
	}

	script := `
		var collator = new Intl.Collator('en');
		var cmp = collator.compare('a', 'b');
		cmp < 0; // 'a' comes before 'b'
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Collator.compare should return negative for 'a' < 'b'")
	}
}

// ===============================================
// 3. Intl.DateTimeFormat (date formatting)
// ===============================================

func TestIntl_DateTimeFormat_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.DateTimeFormat === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.DateTimeFormat: NATIVE")
	} else {
		t.Log("Intl.DateTimeFormat: NOT_AVAILABLE")
		t.Skip("Intl.DateTimeFormat not available")
	}
}

func TestIntl_DateTimeFormat_Format(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.DateTimeFormat === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.DateTimeFormat not available")
	}

	script := `
		var dtf = new Intl.DateTimeFormat('en-US', {
			year: 'numeric',
			month: '2-digit',
			day: '2-digit'
		});
		var formatted = dtf.format(new Date(2026, 1, 6));
		typeof formatted === 'string' && formatted.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DateTimeFormat.format should return non-empty string")
	}
}

// ===============================================
// 4. Intl.NumberFormat (number formatting)
// ===============================================

func TestIntl_NumberFormat_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.NumberFormat === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.NumberFormat: NATIVE")
	} else {
		t.Log("Intl.NumberFormat: NOT_AVAILABLE")
		t.Skip("Intl.NumberFormat not available")
	}
}

func TestIntl_NumberFormat_Format(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.NumberFormat === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.NumberFormat not available")
	}

	script := `
		var nf = new Intl.NumberFormat('en-US', {
			style: 'currency',
			currency: 'USD'
		});
		var formatted = nf.format(1234.56);
		typeof formatted === 'string' && formatted.indexOf('1') !== -1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("NumberFormat.format should return string containing digits")
	}
}

// ===============================================
// 5. Intl.PluralRules (pluralization)
// ===============================================

func TestIntl_PluralRules_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.PluralRules === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.PluralRules: NATIVE")
	} else {
		t.Log("Intl.PluralRules: NOT_AVAILABLE")
		t.Skip("Intl.PluralRules not available")
	}
}

func TestIntl_PluralRules_Select(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.PluralRules === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.PluralRules not available")
	}

	script := `
		var pr = new Intl.PluralRules('en');
		var one = pr.select(1);
		var other = pr.select(5);
		one === 'one' && other === 'other';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("PluralRules.select should return 'one' for 1 and 'other' for 5 in English")
	}
}

// ===============================================
// 6. Intl.RelativeTimeFormat (relative time)
// ===============================================

func TestIntl_RelativeTimeFormat_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.RelativeTimeFormat === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.RelativeTimeFormat: NATIVE")
	} else {
		t.Log("Intl.RelativeTimeFormat: NOT_AVAILABLE")
		t.Skip("Intl.RelativeTimeFormat not available")
	}
}

func TestIntl_RelativeTimeFormat_Format(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.RelativeTimeFormat === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.RelativeTimeFormat not available")
	}

	script := `
		var rtf = new Intl.RelativeTimeFormat('en', { style: 'short' });
		var formatted = rtf.format(-1, 'day');
		typeof formatted === 'string' && formatted.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RelativeTimeFormat.format should return non-empty string")
	}
}

// ===============================================
// 7. Intl.ListFormat (list formatting)
// ===============================================

func TestIntl_ListFormat_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.ListFormat === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.ListFormat: NATIVE")
	} else {
		t.Log("Intl.ListFormat: NOT_AVAILABLE")
		t.Skip("Intl.ListFormat not available")
	}
}

func TestIntl_ListFormat_Format(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.ListFormat === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.ListFormat not available")
	}

	script := `
		var lf = new Intl.ListFormat('en', { style: 'long', type: 'conjunction' });
		var formatted = lf.format(['Apple', 'Banana', 'Cherry']);
		typeof formatted === 'string' && formatted.indexOf('and') !== -1;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ListFormat.format should return string with 'and' for conjunction")
	}
}

// ===============================================
// 8. Intl.Segmenter (text segmentation)
// ===============================================

func TestIntl_Segmenter_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.Segmenter === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.Segmenter: NATIVE")
	} else {
		t.Log("Intl.Segmenter: NOT_AVAILABLE")
		t.Skip("Intl.Segmenter not available")
	}
}

func TestIntl_Segmenter_Segment(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.Segmenter === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.Segmenter not available")
	}

	script := `
		var segmenter = new Intl.Segmenter('en', { granularity: 'word' });
		var segments = segmenter.segment('Hello, World!');
		var count = 0;
		for (var s of segments) {
			count++;
		}
		count > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Segmenter.segment should produce segments")
	}
}

// ===============================================
// 9. Intl.DisplayNames (display name translation)
// ===============================================

func TestIntl_DisplayNames_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.DisplayNames === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.DisplayNames: NATIVE")
	} else {
		t.Log("Intl.DisplayNames: NOT_AVAILABLE")
		t.Skip("Intl.DisplayNames not available")
	}
}

func TestIntl_DisplayNames_Of(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.DisplayNames === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.DisplayNames not available")
	}

	script := `
		var dn = new Intl.DisplayNames('en', { type: 'language' });
		var name = dn.of('fr');
		typeof name === 'string' && name.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("DisplayNames.of should return language name")
	}
}

// ===============================================
// 10. Intl.getCanonicalLocales()
// ===============================================

func TestIntl_GetCanonicalLocales_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.getCanonicalLocales === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.getCanonicalLocales: NATIVE")
	} else {
		t.Log("Intl.getCanonicalLocales: NOT_AVAILABLE")
		t.Skip("Intl.getCanonicalLocales not available")
	}
}

func TestIntl_GetCanonicalLocales_Usage(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.getCanonicalLocales === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.getCanonicalLocales not available")
	}

	script := `
		var locales = Intl.getCanonicalLocales(['EN-us', 'fr-FR']);
		Array.isArray(locales) && locales.length === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("getCanonicalLocales should return array of canonical locale strings")
	}
}

// ===============================================
// 11. Intl.supportedValuesOf()
// ===============================================

func TestIntl_SupportedValuesOf_Exists(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	script := `typeof Intl !== 'undefined' && typeof Intl.supportedValuesOf === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	if v.ToBoolean() {
		t.Log("Intl.supportedValuesOf: NATIVE")
	} else {
		t.Log("Intl.supportedValuesOf: NOT_AVAILABLE")
		t.Skip("Intl.supportedValuesOf not available")
	}
}

func TestIntl_SupportedValuesOf_Usage(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.supportedValuesOf === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.supportedValuesOf not available")
	}

	script := `
		var calendars = Intl.supportedValuesOf('calendar');
		Array.isArray(calendars);
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("supportedValuesOf should return array")
	}
}

// ===============================================
// Comprehensive Status Report
// ===============================================

func TestIntl_StatusReport(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	// Check overall Intl status
	vIntl, _ := runtime.RunString(`typeof Intl`)
	if vIntl.String() == "undefined" {
		t.Log("=== INTL API STATUS REPORT ===")
		t.Log("Intl: NOT_AVAILABLE in Goja")
		t.Log("All Intl APIs require polyfill or external library")
		t.Log("================================")
		return
	}

	t.Log("=== INTL API STATUS REPORT ===")
	t.Logf("Intl object: AVAILABLE (typeof = %s)", vIntl.String())

	// Check each API
	apis := []struct {
		name  string
		check string
	}{
		{"Intl.Collator", `typeof Intl.Collator === 'function'`},
		{"Intl.DateTimeFormat", `typeof Intl.DateTimeFormat === 'function'`},
		{"Intl.NumberFormat", `typeof Intl.NumberFormat === 'function'`},
		{"Intl.PluralRules", `typeof Intl.PluralRules === 'function'`},
		{"Intl.RelativeTimeFormat", `typeof Intl.RelativeTimeFormat === 'function'`},
		{"Intl.ListFormat", `typeof Intl.ListFormat === 'function'`},
		{"Intl.Segmenter", `typeof Intl.Segmenter === 'function'`},
		{"Intl.DisplayNames", `typeof Intl.DisplayNames === 'function'`},
		{"Intl.getCanonicalLocales", `typeof Intl.getCanonicalLocales === 'function'`},
		{"Intl.supportedValuesOf", `typeof Intl.supportedValuesOf === 'function'`},
	}

	for _, api := range apis {
		v, err := runtime.RunString(api.check)
		status := "NOT_AVAILABLE"
		if err == nil && v.ToBoolean() {
			status = "NATIVE"
		}
		t.Logf("%s: %s", api.name, status)
	}
	t.Log("================================")
}

// ===============================================
// Locale Handling Edge Cases
// ===============================================

func TestIntl_LocaleHandling(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	// Check if any Intl API is available to test locale handling
	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.NumberFormat === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.NumberFormat not available")
	}

	// Test invalid locale handling
	script := `
		try {
			new Intl.NumberFormat('invalid-locale-xyz');
			true; // Some implementations may accept any locale fallback
		} catch (e) {
			e instanceof RangeError;
		}
	`
	_, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	t.Log("Locale handling test completed (behavior varies by implementation)")
}

func TestIntl_LocaleNegotiation(t *testing.T) {
	_, runtime, cleanup := newIntlTestAdapter(t)
	defer cleanup()

	vCheck, _ := runtime.RunString(`typeof Intl !== 'undefined' && typeof Intl.NumberFormat === 'function'`)
	if !vCheck.ToBoolean() {
		t.Skip("Intl.NumberFormat not available")
	}

	// Test resolvedOptions which shows locale negotiation result
	script := `
		var nf = new Intl.NumberFormat('en-US');
		var opts = nf.resolvedOptions();
		typeof opts === 'object' && typeof opts.locale === 'string';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("resolvedOptions should return object with locale")
	}
}
