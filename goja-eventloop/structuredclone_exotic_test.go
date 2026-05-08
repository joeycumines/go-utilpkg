package gojaeventloop

import (
	"testing"

	"github.com/joeycumines/goja"
)

type structuredCloneDynamicProbe struct {
	runtime *goja.Runtime
	calls   int
}

func (p *structuredCloneDynamicProbe) Get(string) goja.Value {
	p.calls++
	return p.runtime.ToValue(1)
}

func (p *structuredCloneDynamicProbe) Set(string, goja.Value) bool {
	p.calls++
	return true
}

func (p *structuredCloneDynamicProbe) Has(string) bool {
	p.calls++
	return true
}

func (p *structuredCloneDynamicProbe) Delete(string) bool {
	p.calls++
	return true
}

func (p *structuredCloneDynamicProbe) Keys() []string {
	p.calls++
	return []string{"value"}
}

func TestWebStructuredCloneRejectsECMAScriptExotics(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const factories = [
				() => (function () { return arguments; })(1),
				() => [1][Symbol.iterator](),
				() => new Map([["key", 1]]).entries(),
				() => new Set([1]).values(),
				() => "x"[Symbol.iterator](),
				() => "x".matchAll(/x/g),
				() => (function* () { throw new Error("generator body executed"); })(),
			];
			const outcomes = [];
			for (const factory of factories) {
				for (const nullPrototype of [false, true]) {
					const value = factory();
					if (nullPrototype) Object.setPrototypeOf(value, null);
					for (const input of [value, { value }]) {
						try {
							structuredClone(input);
							outcomes.push("missing");
						} catch (error) {
							outcomes.push(error.name + ":" + error.code);
						}
					}
				}
			}

			const iterator = [1, 2][Symbol.iterator]();
			try { structuredClone(iterator); } catch (_) {}
			const iteratorPosition = iterator.next().value;

			let generatorRuns = 0;
			const generator = (function* () { generatorRuns++; yield 1; })();
			try { structuredClone(generator); } catch (_) {}

			return [
				outcomes.length,
				outcomes.every((result) => result === "DataCloneError:25"),
				iteratorPosition,
				generatorRuns,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "28,true,1,0"; got != want {
		t.Fatalf("exotic structured clone results = %q, want %q", got, want)
	}
}

func TestWebStructuredCloneAcceptsOrdinaryFalseBrands(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const iteratorPrototype = Object.getPrototypeOf([][Symbol.iterator]());
			const fakeIterator = Object.create(iteratorPrototype);
			fakeIterator.marker = 1;
			Object.defineProperty(fakeIterator, Symbol.toStringTag, { value: "Array Iterator" });
			const fakeIteratorClone = structuredClone(fakeIterator);

			class Sample {
				#hidden = 2;
				constructor() { this.visible = 3; }
			}
			const instance = new Sample();
			const instanceClone = structuredClone(instance);

			const nullPrototype = Object.create(null);
			nullPrototype.value = 4;
			const nullPrototypeClone = structuredClone(nullPrototype);

			const generatorPrototype = Object.getPrototypeOf((function* () {})());
			generatorPrototype.marker = 5;
			const generatorPrototypeClone = structuredClone(generatorPrototype);
			delete generatorPrototype.marker;

			const boxedBigInt = Object(6n);
			Object.setPrototypeOf(boxedBigInt, null);
			const boxedBigIntClone = structuredClone(boxedBigInt);

			const boxedSymbol = Object(Symbol("sample"));
			Object.setPrototypeOf(boxedSymbol, null);
			let symbolError = "missing";
			try { structuredClone(boxedSymbol); }
			catch (error) { symbolError = error.name; }

			return [
				fakeIteratorClone.marker,
				Object.getPrototypeOf(fakeIteratorClone) === Object.prototype,
				instanceClone.visible,
				instanceClone instanceof Sample,
				nullPrototypeClone.value,
				Object.getPrototypeOf(nullPrototypeClone) === Object.prototype,
				generatorPrototypeClone.marker,
				boxedBigIntClone.valueOf() === 6n,
				symbolError,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "1,true,3,false,4,true,5,true,DataCloneError"; got != want {
		t.Fatalf("ordinary false-brand clone results = %q, want %q", got, want)
	}
}

func TestWebStructuredCloneRejectsGoHostObjectsWithoutTraversal(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	probe := &structuredCloneDynamicProbe{runtime: adapter.runtime}
	if err := adapter.runtime.Set("dynamicProbe", adapter.runtime.NewDynamicObject(probe)); err != nil {
		t.Fatal(err)
	}
	if err := adapter.runtime.Set("goMapProbe", map[string]int{"value": 1}); err != nil {
		t.Fatal(err)
	}
	if err := adapter.runtime.Set("goSliceProbe", []int{1}); err != nil {
		t.Fatal(err)
	}

	value, err := adapter.runtime.RunString(`
		(() => {
			const outcomes = [];
			for (const value of [dynamicProbe, goMapProbe, goSliceProbe]) {
				for (const input of [value, { value }]) {
					try {
						structuredClone(input);
						outcomes.push("missing");
					} catch (error) {
						outcomes.push(error.name + ":" + error.code);
					}
				}
			}
			return outcomes.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "DataCloneError:25,DataCloneError:25,DataCloneError:25,DataCloneError:25,DataCloneError:25,DataCloneError:25"; got != want {
		t.Fatalf("Go host clone results = %q, want %q", got, want)
	}
	if probe.calls != 0 {
		t.Fatalf("dynamic host object callbacks = %d, want 0", probe.calls)
	}
}
