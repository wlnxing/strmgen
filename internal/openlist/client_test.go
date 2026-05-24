package openlist

import "testing"

func TestHashPassword(t *testing.T) {
	got := HashPassword("password")
	want := "0ee0be47182acad90a4307dd35cc06d901875e870b2637955a1188637ee56675"
	if got != want {
		t.Fatalf("HashPassword() = %s, want %s", got, want)
	}
}

func TestBuildDURL(t *testing.T) {
	got, err := BuildDURL("https://open.example/", "/电影/A B.mkv", "a+b/c", true)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://open.example/d/%E7%94%B5%E5%BD%B1/A%20B.mkv?sign=a%2Bb%2Fc"
	if got != want {
		t.Fatalf("BuildDURL encoded = %s, want %s", got, want)
	}

	got, err = BuildDURL("https://open.example", "/电影/A B.mkv", "", false)
	if err != nil {
		t.Fatal(err)
	}
	want = "https://open.example/d/电影/A B.mkv"
	if got != want {
		t.Fatalf("BuildDURL raw = %s, want %s", got, want)
	}
}
