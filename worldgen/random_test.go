package worldgen

import "testing"

func TestRandomMatchesJava(t *testing.T) {
	// new Random(42): nextInt() is -1170105035, 234785527 and nextInt(100) is 30, 63, 48, 84, 70
	// (checked against an independent port; the first two are the well-known values).
	r := NewRandom(42)
	if a, b := r.Int(), r.Int(); a != -1170105035 || b != 234785527 {
		t.Fatalf("Random(42).nextInt() = %d, %d", a, b)
	}
	r = NewRandom(42)
	for i, want := range []int32{30, 63, 48, 84, 70} {
		if got := r.IntN(100); got != want {
			t.Fatalf("Random(42).nextInt(100) #%d = %d, want %d", i, got, want)
		}
	}
	// new Random(0).nextInt(): -1155484576, -723955400, 1033096058.
	r = NewRandom(0)
	for i, want := range []int32{-1155484576, -723955400, 1033096058} {
		if got := r.Int(); got != want {
			t.Fatalf("Random(0).nextInt() #%d = %d, want %d", i, got, want)
		}
	}
	// A power of two bound takes the high bits: Random(0).nextInt(16) differs from % 16.
	r = NewRandom(0)
	if got := r.IntN(16); got != 11 {
		t.Fatalf("Random(0).nextInt(16) = %d, want 11", got)
	}
	// nextLong and nextDouble of Random(0): -4962768465676381896, 0.730967787376657.
	r = NewRandom(0)
	if got := r.Long(); got != -4962768465676381896 {
		t.Fatalf("Random(0).nextLong() = %d", got)
	}
	r = NewRandom(0)
	if got := r.Double(); got != 0.730967787376657 {
		t.Fatalf("Random(0).nextDouble() = %v", got)
	}
}

func TestJavaHash(t *testing.T) {
	if got := JavaHash("hello"); got != 99162322 {
		t.Fatalf(`"hello".hashCode() = %d`, got)
	}
	if got := JavaHash("gocraft"); got == 0 {
		t.Fatal("zero hash")
	}
}

func TestSinTable(t *testing.T) {
	if mhSin(0) != 0 || mhCos(0) < 0.9999 {
		t.Fatal("sin/cos at 0")
	}
	if got := mhSin(3.14159265 / 2); got < 0.9999 {
		t.Fatalf("sin(pi/2) = %v", got)
	}
}
