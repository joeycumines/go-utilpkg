//go:build unix

package prompt

func (r *Renderer) write(b []byte) {
	if _, err := r.out.Write(b); err != nil {
		panic(err)
	}
}
