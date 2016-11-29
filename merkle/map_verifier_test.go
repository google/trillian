package merkle

import (
	"testing"

	"github.com/google/trillian/crypto"
	"github.com/google/trillian/testonly"
)

func TestVerifyMapInclusionProofWorks(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	for i, tv := range inclusionProofTestVector {
		if err := VerifyMapInclusionProof(h.HashKey(tv.Key), h.HashLeaf(tv.Value), tv.ExpectedRoot, tv.Proof, h); err != nil {
			t.Errorf("(test %d) proof verification failed: %v", i, err)
		}
	}
}

func TestVerifyMapInclusionProofCatchesWrongKey(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	tv := inclusionProofTestVector[0]
	tv.Key = []byte("wibble")
	if err := VerifyMapInclusionProof(h.HashKey(tv.Key), h.HashLeaf(tv.Value), tv.ExpectedRoot, tv.Proof, h); err == nil {
		t.Error("unexpectedly verified proof for incorrect key")
	}
}

func TestVerifyMapInclusionProofCatchesWrongValue(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	tv := inclusionProofTestVector[0]
	tv.Value = []byte("wibble")
	if err := VerifyMapInclusionProof(h.HashKey(tv.Key), h.HashLeaf(tv.Value), tv.ExpectedRoot, tv.Proof, h); err == nil {
		t.Error("unexpectedly verified proof for incorrect value")
	}
}

func TestVerifyMapInclusionProofCatchesWrongRoot(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	tv := inclusionProofTestVector[0]
	tv.ExpectedRoot = h.Digest([]byte("wibble"))
	if err := VerifyMapInclusionProof(h.HashKey(tv.Key), h.HashLeaf(tv.Value), tv.ExpectedRoot, tv.Proof, h); err == nil {
		t.Error("unexpectedly verified proof for incorrect root")
	}
}

func TestVerifyMapInclusionProofCatchesWrongProof(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	tv := inclusionProofTestVector[0]
	tv.Proof[250][15] ^= 0x10
	if err := VerifyMapInclusionProof(h.HashKey(tv.Key), h.HashLeaf(tv.Value), tv.ExpectedRoot, tv.Proof, h); err == nil {
		t.Error("unexpectedly verified proof for incorrect proof")
	}
}

func TestVerifyMapInclusionProofRejectsShortProof(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	err := VerifyMapInclusionProof(h.HashKey([]byte("hi")), h.HashLeaf([]byte("there")), h.Digest([]byte("root")), [][]byte{h.Digest([]byte("shorty"))}, h)
	if err == nil {
		t.Error("unexpectedly verified short proof")
	}
}

func TestVerifyMapInclusionProofRejectsExcess(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	p := make([][]byte, h.Size()*8+1)
	err := VerifyMapInclusionProof(h.HashKey([]byte("hi")), h.HashLeaf([]byte("there")), h.Digest([]byte("root")), p, h)
	if err == nil {
		t.Error("unexpectedly verified proof with extra data")
	}
}

func TestVerifyMapInclusionProofRejectsInvalidKeyHash(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	p := make([][]byte, h.Size()*8)
	err := VerifyMapInclusionProof([]byte("peppo"), h.HashLeaf([]byte("there")), h.Digest([]byte("root")), p, h)
	if err == nil {
		t.Error("unexpectedly verified with invalid key hash")
	}
}

func TestVerifyMapInclusionProofRejectsInvalidLeafHash(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	p := make([][]byte, h.Size()*8)
	err := VerifyMapInclusionProof(h.HashKey([]byte("key")), []byte("peppo"), h.Digest([]byte("root")), p, h)
	if err == nil {
		t.Error("unexpectedly verified with invalid key hash")
	}
}

func TestVerifyMapInclusionProofRejectsInvalidRoot(t *testing.T) {
	h := NewMapHasher(NewRFC6962TreeHasher(crypto.NewSHA256()))
	p := make([][]byte, h.Size()*8)
	err := VerifyMapInclusionProof(h.HashKey([]byte("hi")), h.HashLeaf([]byte("there")), []byte("peppo"), p, h)
	if err == nil {
		t.Error("unexpectedly verified proof with extra data")
	}
}

// Testdata produced with python
var inclusionProofTestVector = []struct {
	Key          []byte
	Value        []byte
	Proof        [][]byte
	ExpectedRoot []byte
}{
	{[]byte("key-0-848"),
		[]byte("value-0-848"),
		[][]byte{
			// 246 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			testonly.MustDecodeBase64("vMWPHFclXXchQbAGJr6pcB002vQZYHnJTfOC42E1iT8="),
			nil,
			testonly.MustDecodeBase64("C3VKkaOliXmuHXM0zrkSulYX6ORaNG8qWHez/dyQkQs="),
			testonly.MustDecodeBase64("7vmVXjPm0XhOMJlnpxJa/ZKn8eeK0PIthOOy74w+sJc="),
			testonly.MustDecodeBase64("vEWXkf+9ZJQ/oxyyOaQdIfZfsx2GCA/NldZ+UopQF6Y="),
			testonly.MustDecodeBase64("lrGGFxtBKRdE53Dl6p0GeFgM6VomF9Fx5k/6+aIzMWc="),
			testonly.MustDecodeBase64("I5nVuy9wljpxbgv/aE9ivo854GhFRdsAWwmmEXDjaxE="),
			testonly.MustDecodeBase64("yAxifDRQUd+vjc6RaHG9f8tCWSa0mzV4rry50khiD3M="),
			testonly.MustDecodeBase64("YmUpJx/UagsoBYv6PnFRaVYw3x6kAx3N3OOSyiXsGtg="),
			testonly.MustDecodeBase64("CtC2GCsc3/zFn1DNkoUThUnn7k+DMotaNXvmceKIL4Y="),
		},
		testonly.MustDecodeBase64("U6ANU1en3BSbbnWqhV2nTGtQ+scBlaZf9kRPEEDZsHM="),
	}, {
		[]byte("key-1-848"),
		[]byte("value-1-848"),
		[][]byte{
			// 246 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil,
			testonly.MustDecodeBase64("bKUeTeIo5yNgZOJvU+MpfTQQlgOCasII6lglAg9BKXY="),
			testonly.MustDecodeBase64("H35adeuKvGi/1cTAu1bxMFfFXwItLiKhggRdHE/uyro="),
			testonly.MustDecodeBase64("rUKL1gJQQZVwYrQBxjxrYO/3P/Q6FDSetGqNwRxyk5A="),
			testonly.MustDecodeBase64("8dg2BbkzWzeNBB0JSHuf6x5XcTyW2x2/b4KapNdYACc="),
			testonly.MustDecodeBase64("jm1Z2OcD+n6eHcMDx73qWKZsT/LhzKTsdU1TCGfGaIY="),
			testonly.MustDecodeBase64("9O6P4cOx1OqmHAg/tiAbdA/0x/bBFtovW5BfoRcrY5o="),
			testonly.MustDecodeBase64("z55TNrWY+8Wgv4a4u4NEqwdCytwp8/HjT3D0BNIEFTI="),
			testonly.MustDecodeBase64("xQzWi9r+QBAjMNpE+gN+l5GtIhHlYP2NXrBC8kUU/EY="),
			testonly.MustDecodeBase64("zY5ONepiFwzAo102WU6DzMRApJKAe/dwOhns+guFAkg="),
			testonly.MustDecodeBase64("9KJS1KgpfzbiVyaKKFFzLuKROQ/hAPsPvii0+/Us8+I="),
		},
		testonly.MustDecodeBase64("gdpe8FlS2bUYksxb392j+v/qxyYwUZC9uj1Lm62r3d0="),
	}, {
		[]byte("key-2-848"),
		[]byte("value-2-848"),
		[][]byte{
			// 244 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil,
			testonly.MustDecodeBase64("t1OxaGjUkK+5sBsbgx5magZdh4VShTzSNvI4N925vFs="),
			nil,
			testonly.MustDecodeBase64("caC5J6720XiPfIcha76Os2x27TKw3OiGgu7vHc1SFuk="),
			testonly.MustDecodeBase64("9fNicrw1ROQi2f/Ltcvbgj6vfz+TTdWM378cDR8WWxg="),
			testonly.MustDecodeBase64("f+hTbDmbASIMv6DljlYjvBJ7m8rla5Z8T6zxkTasv/M="),
			testonly.MustDecodeBase64("GBODQoymwvfJAJSLvxCrS27gOPtrVLfJJVYNfxmPknE="),
			testonly.MustDecodeBase64("LqSHvjmOrF2Cf1aFXCYN3kMqWbu7aNo1C15qDXKwfFU="),
			testonly.MustDecodeBase64("nj+mho3MS9RZq29+QWcHlCJLMGi1IAMPnWXopf8QoKw="),
			testonly.MustDecodeBase64("QDsAIhiw3+GASUAdNMTW/fVRbv67fwiNrtzFKm4cpGU="),
			testonly.MustDecodeBase64("aTqVP3hCyRHNUaP7hemCQ46oS1qfwAuRCWQnqwR3RsU="),
			testonly.MustDecodeBase64("2jpxzwUIUeOpBbIMFmOXGCGA54kB/pvcO+kok2cqiKM="),
			testonly.MustDecodeBase64("aeyl7vP9KXhfLo0GoIULaKsXDqEDb7HhKYJBTc+JW4I="),
		},
		testonly.MustDecodeBase64("vfV8k7xAvSNHvDp7lmKjppWlXlYh3ftq801B1QmFMA8="),
	}, {
		[]byte("key-3-848"),
		[]byte("value-3-848"),
		[][]byte{
			// 236 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			testonly.MustDecodeBase64("TCoz0L9ZdVAfXzB5DYi7JkvgSu41TJHFk1z4z+aRrYM="),
			nil, nil, nil, nil, nil, nil,
			testonly.MustDecodeBase64("H/5CFPaHvNYyq2vvbgwgUnpRjbvPFgoQMh+0lwKyWmE="),
			testonly.MustDecodeBase64("Q0DymzR94JVkVb3djyW5e6lSN4ZF5yRHFKVHQJ8abFE="),
			testonly.MustDecodeBase64("hCsLGJGsNOtD17I5wTfhMgVXXg9PplDuYHohEX0uvZI="),
			testonly.MustDecodeBase64("IMQmP0/VI+2jNh1sQtSotTtIRjHmWrXABSyXZ9U9vUo="),
			testonly.MustDecodeBase64("KnIt+BwTybVh6GhYqnh2tGeGvseXUoZtvR3YP5fBF6s="),
			testonly.MustDecodeBase64("Hth6J7YLf+4WL6XKX+sRkKTj9ME/2gmrkB3CWCV3HSY="),
			testonly.MustDecodeBase64("tGbuTo30YPbdgaU0614qRofiO9hMNh/jsTai9/HicMk="),
			testonly.MustDecodeBase64("uujeGLnv42VsXWm4xR1iNGFi0LazL81V2zuhoCV+xwE="),
			testonly.MustDecodeBase64("HEq0JbLal4nVVYeK8jgzVxO75uRoJ7hMQUZLy8snkig="),
			testonly.MustDecodeBase64("PmccqijdN1j6S25Tn22HNiysdaNwhX4aPG8x7JSRfc4="),
			testonly.MustDecodeBase64("t6UeSSZj9dEov5Vhll2x8b/266IQngg5XgyTHjPbsPQ="),
			testonly.MustDecodeBase64("U2nveNA/h2WQZ7mEmeWknn6phAIlJ4pxYw+U+1ZXSmQ="),
			testonly.MustDecodeBase64("wDB5KKmLMkH4eWtv9HHb8SmatjP4kjYuKW14nQI+yAk="),
		},
		testonly.MustDecodeBase64("7LkdcssuSGAv+6BrLXhUXZbuxWepuGy6ZaJe2JcHy2w="),
	}, {
		[]byte("key-4-848"),
		[]byte("value-4-848"),
		[][]byte{
			// 243 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
			testonly.MustDecodeBase64("RMUW9pcA1aAv4UgU8P5m7SrD0b3CCcO+Bhc0T/AfYsc="),
			testonly.MustDecodeBase64("r+tZ2h/t75Ef1KXh1NaEAx7K2WDrhOHwoa4qJIcsYCw="),
			testonly.MustDecodeBase64("lytZmQPjKcQ6ylWru5U/j6TPq9OtEgBO7UAkEeJt8+s="),
			testonly.MustDecodeBase64("UDbur6hehAiic3ov1VXWWafqdwGD35gSEhC1OD8t8YE="),
			testonly.MustDecodeBase64("BgD8awFsMcQJYZfIe1PhqkiEDH1fo/WCDcsoZ3szVNE="),
			testonly.MustDecodeBase64("QJCGI/QhjajxLJdEEqR9JBylBa65AuN2ppvY+6qFmX0="),
			testonly.MustDecodeBase64("Baws8Zc/6ql08L7m4alVJqo7bske1EhRteNVEUEEaZA="),
			testonly.MustDecodeBase64("us0ShL/Ly3Jp5Vxx6jutAHEWVl1H2MKYkUNyeJjOg4Q="),
			testonly.MustDecodeBase64("aDtxuuyTm/u1BgLvVzCQt5rYY7of9o2l5ovLc5Lp4rA="),
			testonly.MustDecodeBase64("Eb3b6jxsqEjO2YKcsWE7exMDP15QVohkwRDaNO7QOwo="),
			testonly.MustDecodeBase64("M/Q7RQtMDsE2bok/JxYwc4Rco3H0Juil1XfmRWtbbUw="),
			testonly.MustDecodeBase64("QLSisPLWbV8FjwrFnUJxqyMKDnof//Djjrq082btVsQ="),
			testonly.MustDecodeBase64("UoDpJ8sv0EymM0xNz/gUHB1icfS67wE5m+lUDGVI6Ic="),
		},
		testonly.MustDecodeBase64("bEapbZbXfyhAZqLAFPpbx2KMX/m9FvHenwFJFIn2LHs="),
	}, {
		[]byte("key-4-848"),
		[]byte("value-4-848"),
		[][]byte{
			// 243 x nil
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil,
			testonly.MustDecodeBase64("RMUW9pcA1aAv4UgU8P5m7SrD0b3CCcO+Bhc0T/AfYsc="),
			testonly.MustDecodeBase64("r+tZ2h/t75Ef1KXh1NaEAx7K2WDrhOHwoa4qJIcsYCw="),
			testonly.MustDecodeBase64("lytZmQPjKcQ6ylWru5U/j6TPq9OtEgBO7UAkEeJt8+s="),
			testonly.MustDecodeBase64("UDbur6hehAiic3ov1VXWWafqdwGD35gSEhC1OD8t8YE="),
			testonly.MustDecodeBase64("BgD8awFsMcQJYZfIe1PhqkiEDH1fo/WCDcsoZ3szVNE="),
			testonly.MustDecodeBase64("QJCGI/QhjajxLJdEEqR9JBylBa65AuN2ppvY+6qFmX0="),
			testonly.MustDecodeBase64("Baws8Zc/6ql08L7m4alVJqo7bske1EhRteNVEUEEaZA="),
			testonly.MustDecodeBase64("us0ShL/Ly3Jp5Vxx6jutAHEWVl1H2MKYkUNyeJjOg4Q="),
			testonly.MustDecodeBase64("aDtxuuyTm/u1BgLvVzCQt5rYY7of9o2l5ovLc5Lp4rA="),
			testonly.MustDecodeBase64("Eb3b6jxsqEjO2YKcsWE7exMDP15QVohkwRDaNO7QOwo="),
			testonly.MustDecodeBase64("M/Q7RQtMDsE2bok/JxYwc4Rco3H0Juil1XfmRWtbbUw="),
			testonly.MustDecodeBase64("QLSisPLWbV8FjwrFnUJxqyMKDnof//Djjrq082btVsQ="),
			testonly.MustDecodeBase64("UoDpJ8sv0EymM0xNz/gUHB1icfS67wE5m+lUDGVI6Ic="),
		},
		testonly.MustDecodeBase64("bEapbZbXfyhAZqLAFPpbx2KMX/m9FvHenwFJFIn2LHs="),
	},
}
