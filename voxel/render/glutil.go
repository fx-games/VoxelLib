// Package render draws voxel.World meshes with OpenGL 3.3 core. It needs a
// current GL context on the calling goroutine (the main thread with GLFW).
package render

import (
	"fmt"
	"strings"

	"github.com/memmaker/voxellib/platform/gl"
)

func compile(src string, kind uint32) (uint32, error) {
	s := gl.CreateShader(kind)
	cs, free := gl.Strs(src + "\x00")
	gl.ShaderSource(s, 1, cs, nil)
	free()
	gl.CompileShader(s)
	var ok int32
	gl.GetShaderiv(s, gl.COMPILE_STATUS, &ok)
	if ok == gl.FALSE {
		var n int32
		gl.GetShaderiv(s, gl.INFO_LOG_LENGTH, &n)
		log := strings.Repeat("\x00", int(n+1))
		gl.GetShaderInfoLog(s, n, nil, gl.Str(log))
		gl.DeleteShader(s)
		return 0, fmt.Errorf("shader compile: %s", log)
	}
	return s, nil
}

// program links a vertex+fragment shader pair.
type program struct {
	id       uint32
	uniforms map[string]int32
}

func newProgram(vs, fs string) (*program, error) {
	v, err := compile(vs, gl.VERTEX_SHADER)
	if err != nil {
		return nil, err
	}
	f, err := compile(fs, gl.FRAGMENT_SHADER)
	if err != nil {
		return nil, err
	}
	p := gl.CreateProgram()
	gl.AttachShader(p, v)
	gl.AttachShader(p, f)
	gl.LinkProgram(p)
	gl.DeleteShader(v)
	gl.DeleteShader(f)
	var ok int32
	gl.GetProgramiv(p, gl.LINK_STATUS, &ok)
	if ok == gl.FALSE {
		var n int32
		gl.GetProgramiv(p, gl.INFO_LOG_LENGTH, &n)
		log := strings.Repeat("\x00", int(n+1))
		gl.GetProgramInfoLog(p, n, nil, gl.Str(log))
		return nil, fmt.Errorf("program link: %s", log)
	}
	return &program{id: p, uniforms: map[string]int32{}}, nil
}

func (p *program) loc(name string) int32 {
	if l, ok := p.uniforms[name]; ok {
		return l
	}
	l := gl.GetUniformLocation(p.id, gl.Str(name+"\x00"))
	p.uniforms[name] = l
	return l
}

func (p *program) use() { gl.UseProgram(p.id) }
