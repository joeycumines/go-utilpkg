package gojaeventloop

import "testing"

func TestStructuredClone_BufferSourcesAndTransfer(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var buf = new ArrayBuffer(8);
			var bytes = new Uint8Array(buf);
			bytes.set([9, 8, 7, 6, 5, 4, 3, 2]);

			var clonedBuffer = structuredClone(buf);
			var clonedBytes = new Uint8Array(clonedBuffer);
			var arrayBufferOK = clonedBuffer !== buf && clonedBytes[0] === 9 && clonedBytes[3] === 6;
			bytes[0] = 1;
			arrayBufferOK = arrayBufferOK && clonedBytes[0] === 9;

			var view = new Uint8Array(buf, 2, 4);
			var clonedView = structuredClone(view);
			var typedArrayOK = clonedView instanceof Uint8Array &&
				clonedView !== view &&
				clonedView.buffer !== view.buffer &&
				clonedView.byteOffset === 2 &&
				clonedView.length === 4 &&
				clonedView[0] === 7;
			view[0] = 44;
			typedArrayOK = typedArrayOK && clonedView[0] === 7;

			var dataView = new DataView(buf, 2, 4);
			dataView.setUint8(1, 33);
			var clonedDataView = structuredClone(dataView);
			var dataViewOK = clonedDataView instanceof DataView &&
				clonedDataView !== dataView &&
				clonedDataView.buffer !== dataView.buffer &&
				clonedDataView.byteOffset === 2 &&
				clonedDataView.byteLength === 4 &&
				clonedDataView.getUint8(1) === 33;

			var transferBuffer = new ArrayBuffer(4);
			new Uint8Array(transferBuffer).set([1, 2, 3, 4]);
			var transferred = structuredClone({ buffer: transferBuffer }, { transfer: [transferBuffer] });
			var transferredBytes = new Uint8Array(transferred.buffer);
			var transferOK = transferBuffer.byteLength === 0 &&
				transferred.buffer.byteLength === 4 &&
				transferredBytes[0] === 1 && transferredBytes[3] === 4;

			return arrayBufferOK && typedArrayOK && dataViewOK && transferOK;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Fatalf("buffer source / transfer structuredClone checks failed")
	}
}

func TestStructuredClone_TransferDetachedAndUnreachableBuffers(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var out = [];
			var unreachable = new ArrayBuffer(1);
			structuredClone({ ok: true }, { transfer: [unreachable] });
			out.push("unreachable-detached:" + unreachable.byteLength);

			var detachedGets = 0;
			try {
				structuredClone({
					get probe() { detachedGets++; return true; },
				}, { transfer: [unreachable] });
				out.push("detached:ok");
			} catch (e) {
				out.push("detached:" + e.name + ":" + e.constructor.name + ":" + e.message + ":gets-" + detachedGets);
			}

			var reachable = new ArrayBuffer(2);
			new Uint8Array(reachable).set([7, 8]);
			var cloned = structuredClone({ buffer: reachable }, { transfer: [reachable] });
			out.push("reachable:" + reachable.byteLength + ":" + Array.from(new Uint8Array(cloned.buffer)).join("-"));
			return out.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := result.String(), "unreachable-detached:0,detached:DataCloneError:DOMException:Cannot transfer object of unsupported type.:gets-1,reachable:0:7-8"; got != want {
		t.Fatalf("structuredClone transfer detached/unreachable = %q, want %q", got, want)
	}
}

func TestStructuredClone_TransferTypeTaxonomy(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function () {
			function observe(run) {
				try { run(); return "missing"; }
				catch (error) {
					return [error.constructor.name, error.name, String(error.code), error instanceof DOMException].join(":");
				}
			}
			return [
				observe(() => structuredClone({}, { transfer: 1 })),
				observe(() => structuredClone({}, { transfer: [1] })),
				observe(() => structuredClone({}, { transfer: [{}] })),
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "TypeError:TypeError:undefined:false," +
		"TypeError:TypeError:undefined:false," +
		"DOMException:DataCloneError:25:true"
	if got := result.String(); got != want {
		t.Fatalf("structuredClone transfer taxonomy = %q, want %q", got, want)
	}
}

func TestStructuredClone_TransferTraversalPrecedesDetachedCheck(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(() => {
			const buffer = new ArrayBuffer(1);
			let laterGets = 0;
			let observed = "missing";
			try {
				structuredClone({
					get detach() {
						structuredClone(buffer, { transfer: [buffer] });
						return true;
					},
					buffer,
					get later() { laterGets++; return true; },
				}, { transfer: [buffer] });
			} catch (error) {
				observed = error.name;
			}
			return [observed, laterGets, buffer.byteLength].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := result.String(), "DataCloneError,1,0"; got != want {
		t.Fatalf("detached transfer traversal = %q, want %q", got, want)
	}
}

func TestStructuredClone_TransferFailurePreservesListOrder(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(() => {
			function run(laterFirst) {
				const earlier = new ArrayBuffer(1);
				const later = new ArrayBuffer(1);
				let observed = "missing";
				try {
					structuredClone({
						get detachLater() {
							structuredClone(later, { transfer: [later] });
							return true;
						},
					}, { transfer: laterFirst ? [later, earlier] : [earlier, later] });
				} catch (error) {
					observed = error.name;
				}
				return [observed, earlier.byteLength, later.byteLength].join("/");
			}
			return [run(false), run(true)].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := result.String(), "DataCloneError/0/0,DataCloneError/1/0"; got != want {
		t.Fatalf("ordered transfer failure = %q, want %q", got, want)
	}
}

func TestStructuredClone_TransferPreservesGraphAndViewIdentity(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(() => {
			const first = new ArrayBuffer(6);
			const firstBytes = new Uint8Array(first);
			firstBytes.set([1, 2, 3, 4, 5, 6]);
			const firstDataView = new DataView(first, 1, 3);
			const firstSubview = new Uint8Array(first, 2, 2);

			const second = new ArrayBuffer(3);
			const secondView = new Uint8Array(second);
			secondView.set([7, 8, 9]);

			const source = {
				first,
				firstAlias: first,
				firstDataView,
				firstSubview,
				firstViewAlias: firstSubview,
				second,
				secondView,
			};
			source.self = source;
			const cloned = structuredClone(source, { transfer: [first, second] });

			function detached(view) {
				try { return view.byteLength === 0; }
				catch (error) { return error instanceof TypeError; }
			}
			return [
				cloned.self === cloned,
				cloned.first === cloned.firstAlias,
				cloned.firstDataView.buffer === cloned.first,
				cloned.firstSubview.buffer === cloned.first,
				cloned.firstSubview === cloned.firstViewAlias,
				cloned.firstDataView.byteOffset,
				cloned.firstDataView.byteLength,
				cloned.firstSubview.byteOffset,
				cloned.firstSubview.length,
				Array.from(new Uint8Array(cloned.first)).join("/"),
				cloned.secondView.buffer === cloned.second,
				cloned.second !== cloned.first,
				Array.from(cloned.secondView).join("/"),
				first.byteLength,
				second.byteLength,
				detached(firstDataView),
				detached(firstSubview),
				detached(secondView),
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	const want = "true,true,true,true,true,1,3,2,2,1/2/3/4/5/6,true,true,7/8/9,0,0,true,true,true"
	if got := result.String(); got != want {
		t.Fatalf("transferred graph identity = %q, want %q", got, want)
	}
}

func TestStructuredClone_ThrowingTransferIteratorDetachesNothing(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(() => {
			const first = new ArrayBuffer(1);
			const second = new ArrayBuffer(1);
			const thrown = { marker: "iterator" };
			const transfer = {
				[Symbol.iterator]() {
					let step = 0;
					return {
						next() {
							if (step++ === 0) return { value: first, done: false };
							throw thrown;
						},
					};
				},
			};
			let sameError = false;
			try { structuredClone({ first, second }, { transfer }); }
			catch (error) { sameError = error === thrown; }
			return [sameError, first.byteLength, second.byteLength].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := result.String(), "true,1,1"; got != want {
		t.Fatalf("throwing transfer iterator = %q, want %q", got, want)
	}
}
