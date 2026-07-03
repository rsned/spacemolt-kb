package cubemap

import (
	"image/color"
	"testing"
)

func TestNewCubeMap(t *testing.T) {
	cm := New(64)
	if cm.Size != 64 {
		t.Fatalf("Size = %d, want 64", cm.Size)
	}
	for i, f := range cm.Faces {
		if len(f) != 64*64 {
			t.Errorf("Faces[%d] len = %d, want %d", i, len(f), 64*64)
		}
	}
}

func TestNewCubeMapF(t *testing.T) {
	cm := NewF(64)
	if cm.Size != 64 {
		t.Fatalf("Size = %d, want 64", cm.Size)
	}
	for i, f := range cm.Faces {
		if len(f) != 64*64 {
			t.Errorf("Faces[%d] len = %d, want %d", i, len(f), 64*64)
		}
	}
}

func TestCubeMapSetGet(t *testing.T) {
	cm := New(8)
	red := color.RGBA{R: 255, A: 255}
	cm.Set(FacePosX, 3, 5, red)
	got := cm.Get(FacePosX, 3, 5)
	if got != red {
		t.Errorf("Get returned %v, want %v", got, red)
	}
}

func TestCubeMapFSetGet(t *testing.T) {
	cm := NewF(8)
	cm.Set(FaceNegZ, 1, 7, 0.42)
	got := cm.Get(FaceNegZ, 1, 7)
	if got != 0.42 {
		t.Errorf("Get returned %f, want 0.42", got)
	}
}

func TestFaceConstants(t *testing.T) {
	if FacePosX != 0 || FaceNegX != 1 || FacePosY != 2 ||
		FaceNegY != 3 || FacePosZ != 4 || FaceNegZ != 5 {
		t.Fatalf("face constants not in GL order")
	}
	if NumFaces != 6 {
		t.Fatalf("NumFaces = %d, want 6", NumFaces)
	}
}
