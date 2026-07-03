// Package cubemap provides a 6-faced cube-map storage and sampling
// substrate for sphere-native planet generation. Each face is a square
// 2D grid of pixels; samples on the unit sphere map to a face index
// plus (u, v) coordinates following the OpenGL GL_TEXTURE_CUBE_MAP
// convention.
//
// CubeMap holds RGBA values; CubeMapF holds float64 values for
// scalar fields like heightmaps and distance maps.
//
// On-disk format is a 4×3 horizontal cross PNG (4S × 3S pixels for
// face size S) with empty cells transparent.
package cubemap
