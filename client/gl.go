package client

import (
	"fmt"
	"strings"

	"github.com/go-gl/gl/v3.3-core/gl"
)

const worldVS = `#version 330 core
layout(location = 0) in vec3 pos;
layout(location = 1) in vec2 uv;
layout(location = 2) in vec4 col;
uniform mat4 mvp;
uniform vec4 dyn[16];   // dynamic lights: world position and level
uniform int dynCount;
out vec2 vUV;
out vec3 vCol;
out vec2 vLight;        // block light, sky light, in levels 0..15
out float vDist;
void main() {
	gl_Position = mvp * vec4(pos, 1.0);
	vUV = uv;
	vCol = col.rgb;
	float lb = floor(col.a * 255.0 + 0.5);
	float sky = floor(lb / 16.0);
	float blk = lb - sky * 16.0;
	for (int i = 0; i < dynCount; i++) {
		blk = max(blk, dyn[i].w - distance(pos, dyn[i].xyz));
	}
	vLight = vec2(blk, sky);
	vDist = length(gl_Position.xyz);
}`

const worldFS = `#version 330 core
in vec2 vUV;
in vec3 vCol;
in vec2 vLight;
in float vDist;
uniform sampler2D atlas;
uniform sampler2D lightmap;
uniform vec3 fogColor;
uniform float fogStart;
uniform float fogEnd;
uniform float alpha;
out vec4 frag;
void main() {
	vec4 t = texture(atlas, vUV);
	if (t.a < 0.1) discard;
	vec3 light = texture(lightmap, (clamp(vLight, 0.0, 15.0) + 0.5) / 16.0).rgb;
	float f = clamp((vDist - fogStart) / (fogEnd - fogStart), 0.0, 1.0);
	frag = vec4(mix(t.rgb * vCol * light, fogColor, f), alpha);
}`

const flatVS = `#version 330 core
layout(location = 0) in vec2 pos;
void main() { gl_Position = vec4(pos, 0.0, 1.0); }`

const flatFS = `#version 330 core
uniform vec4 colour;
out vec4 frag;
void main() { frag = colour; }`

func compile(kind uint32, src string) (uint32, error) {
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
		log := strings.Repeat("\x00", int(n)+1)
		gl.GetShaderInfoLog(s, n, nil, gl.Str(log))
		return 0, fmt.Errorf("shader: %s", log)
	}
	return s, nil
}

func program(vs, fs string) (uint32, error) {
	v, err := compile(gl.VERTEX_SHADER, vs)
	if err != nil {
		return 0, err
	}
	f, err := compile(gl.FRAGMENT_SHADER, fs)
	if err != nil {
		return 0, err
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
		log := strings.Repeat("\x00", int(n)+1)
		gl.GetProgramInfoLog(p, n, nil, gl.Str(log))
		return 0, fmt.Errorf("link: %s", log)
	}
	return p, nil
}

func uniform(p uint32, name string) int32 { return gl.GetUniformLocation(p, gl.Str(name+"\x00")) }

// The sky: a gradient from the colour overhead to the fog colour at the horizon, drawn behind the
// world from the direction each pixel looks in.
const skyVS = `#version 330 core
out vec2 ndc;
void main() {
	vec2 p = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);
	ndc = p * 2.0 - 1.0;
	gl_Position = vec4(ndc, 1.0, 1.0);
}`

const skyFS = `#version 330 core
in vec2 ndc;
uniform vec3 forward;
uniform vec3 right;
uniform vec3 up;
uniform vec2 tanFov;
uniform vec3 skyTop;
uniform vec3 skyHorizon;
out vec4 frag;
void main() {
	vec3 d = normalize(forward + right * (ndc.x * tanFov.x) + up * (ndc.y * tanFov.y));
	float t = clamp(d.y * 1.6, 0.0, 1.0);
	frag = vec4(mix(skyHorizon, skyTop, pow(t, 0.7)), 1.0);
}`
