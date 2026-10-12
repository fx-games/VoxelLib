package render

const terrainVS = `#version 330 core
layout(location = 0) in uvec4 aData;

uniform mat4 uViewProj;     // rotation-only view * projection (camera-relative)
uniform vec3 uRegionOrigin; // region origin minus camera position

out vec3 vUV;
out vec3 vColor;
out vec2 vLight;
out float vDist;

const float SHADE[7] = float[7](0.5, 1.0, 0.8, 0.8, 0.6, 0.6, 1.0);
const float AO[4] = float[4](0.45, 0.65, 0.82, 1.0);

void main() {
	uint w0 = aData.x, w1 = aData.y, w2 = aData.z, w3 = aData.w;
	vec3 local = vec3(float(w0 & 1023u), float((w0 >> 10) & 1023u), float((w0 >> 20) & 1023u)) * (1.0 / 32.0);
	uint si = w2 >> 24;
	vec3 sec = vec3(float(si & 7u), float((si >> 3) & 3u), float((si >> 5) & 7u)) * 16.0;
	vec3 pos = uRegionOrigin + sec + local;
	gl_Position = uViewProj * vec4(pos, 1.0);

	vUV = vec3(float((w1 >> 12) & 31u), float((w1 >> 17) & 31u), float(w1 & 4095u));
	uint n = (w1 >> 22) & 7u;
	vec3 tint = vec3(float(w2 & 255u), float((w2 >> 8) & 255u), float((w2 >> 16) & 255u)) * (1.0 / 255.0);
	vColor = tint * SHADE[n] * AO[w0 >> 30];
	vLight = vec2(float(w3 & 255u), float((w3 >> 8) & 255u)) * (1.0 / 240.0);
	vDist = length(pos.xz); // cylindrical fog like Minecraft
}
`

const terrainFS = `#version 330 core
in vec3 vUV;
in vec3 vColor;
in vec2 vLight;
in float vDist;

uniform sampler2DArray uTex;
uniform float uDaylight;
uniform vec3 uFogColor;
uniform vec2 uFog;  // start, end
uniform float uAlphaCut;

out vec4 fragColor;

// Minecraft's lightmap brightness curve with a small ambient floor.
float curve(float l) {
	return mix(0.04, 1.0, l / (4.0 - 3.0 * l));
}

void main() {
	vec4 c = texture(uTex, vUV);
	if (c.a < uAlphaCut) discard;
	float sky = curve(clamp(vLight.x, 0.0, 1.0)) * uDaylight;
	float blk = curve(clamp(vLight.y, 0.0, 1.0));
	vec3 light = max(vec3(sky) * vec3(0.95, 0.97, 1.0), vec3(blk) * vec3(1.0, 0.86, 0.66));
	light = max(light, vec3(0.04));
	vec3 col = c.rgb * vColor * light;
	float f = clamp((vDist - uFog.x) / (uFog.y - uFog.x), 0.0, 1.0);
	col = mix(col, uFogColor, f * f);
	fragColor = vec4(col, c.a);
}
`

const skyVS = `#version 330 core
out vec2 vNDC;
void main() {
	vec2 p = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2)) * 2.0 - 1.0;
	vNDC = p;
	gl_Position = vec4(p, 0.9999, 1.0);
}
`

const skyFS = `#version 330 core
in vec2 vNDC;
uniform mat4 uInvViewProj;
uniform vec3 uFogColor;
uniform vec3 uSkyColor;
uniform vec3 uSunDir;
out vec4 fragColor;
void main() {
	vec4 p = uInvViewProj * vec4(vNDC, 1.0, 1.0);
	vec3 dir = normalize(p.xyz / p.w);
	float h = clamp(dir.y, -0.2, 1.0);
	vec3 col = mix(uFogColor, uSkyColor, smoothstep(0.0, 0.45, h));
	float sun = max(dot(dir, uSunDir), 0.0);
	col += vec3(1.0, 0.95, 0.8) * step(0.9985, sun);
	col += vec3(1.0, 0.8, 0.5) * pow(sun, 64.0) * 0.25;
	fragColor = vec4(col, 1.0);
}
`

const lineVS = `#version 330 core
layout(location = 0) in vec3 aPos;
uniform mat4 uMVP;
void main() { gl_Position = uMVP * vec4(aPos, 1.0); }
`

const lineFS = `#version 330 core
uniform vec4 uColor;
out vec4 fragColor;
void main() { fragColor = uColor; }
`
