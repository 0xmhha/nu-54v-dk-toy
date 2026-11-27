package commerce

import (
	"testing"
	"time"
)

// A challenge says what it was given for, and only a challenge this process gave is read back: any other 32 bytes,
// and a challenge with one bit changed, are not.
func TestChallengeCarriesItsPurposeUnderTheServersTag(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var keys challengeKeys
	order := "6f1c1a52-4c39-4d1e-9d36-1f0b9b4d2a10"
	expires := now.Add(2 * time.Minute)
	for purpose, subject := range map[string]string{"checkout": branch, "order": order, "pickup": order, "account": ""} {
		given, err := keys.mint(now, purpose, subject, expires)
		if err != nil {
			t.Fatal(purpose, err)
		}
		read, genuine := keys.open(now, given)
		if !genuine || read.purpose != purpose || read.subject != subject || !read.expires.Equal(expires) {
			t.Fatalf("a %s challenge reads back as %+v (genuine %v)", purpose, read, genuine)
		}
		// Every bit is under the tag: the purpose, the expiry, the store or order, the random part and the tag.
		for bit := 0; bit < 256; bit++ {
			changed := given
			changed[bit/8] ^= 1 << (bit % 8)
			if _, genuine := keys.open(now, changed); genuine {
				t.Fatalf("a %s challenge with bit %d changed is still read", purpose, bit)
			}
		}
		// The purpose cannot be exchanged for another, whatever the rest says.
		for other := byte(0); other < 8; other++ {
			if swapped := given; other != given[0] {
				swapped[0] = other
				if _, genuine := keys.open(now, swapped); genuine {
					t.Fatalf("a %s challenge is read with purpose code %d", purpose, other)
				}
			}
		}
		// Two challenges for the same thing in the same second differ.
		if again, _ := keys.mint(now, purpose, subject, expires); again == given {
			t.Fatalf("two %s challenges of one second are the same", purpose)
		}
	}
	if _, genuine := keys.open(now, [32]byte{}); genuine {
		t.Fatal("32 zero bytes are read as a challenge")
	}
	if _, err := keys.mint(now, "orders", branch, expires); err == nil {
		t.Fatal("a challenge was made for a purpose there is none of")
	}
	for _, reference := range []string{"", "short", "not-a-reference-of-36-characters!!!!"} {
		if _, err := keys.mint(now, "checkout", reference, expires); err == nil {
			t.Fatalf("a challenge was made for the store reference %q", reference)
		}
	}
	// Another process has another key: after a restart no challenge given before is read.
	var restarted challengeKeys
	given, _ := keys.mint(now, "checkout", branch, expires)
	if _, genuine := restarted.open(now, given); genuine {
		t.Fatal("a challenge of another process is read")
	}
}

// The key changes every ten minutes and the one before it is kept for ten more, so a challenge given just before a
// change is still read for as long as it lives, and one from two keys back is not.
func TestChallengeKeyChanges(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	var keys challengeKeys
	first, err := keys.mint(now, "checkout", branch, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	original := string(keys.current)
	now = now.Add(challengeKeyLife - time.Second)
	last, _ := keys.mint(now, "checkout", branch, now.Add(2*time.Minute)) // one second before the change
	if string(keys.current) != original {
		t.Fatal("the key changed before its time")
	}
	now = now.Add(2 * time.Second)
	next, _ := keys.mint(now, "checkout", branch, now.Add(2*time.Minute))
	if string(keys.current) == original || string(keys.previous) != original {
		t.Fatal("the key did not change after its time, or the one before it was not kept")
	}
	for name, given := range map[string][32]byte{"given under the first key": first, "given a second before the change": last, "given under the new key": next} {
		if _, genuine := keys.open(now, given); !genuine {
			t.Fatalf("a challenge %s is not read after one change", name)
		}
	}
	now = now.Add(challengeKeyLife)
	if _, genuine := keys.open(now, last); genuine {
		t.Fatal("a challenge from two keys back is still read")
	}
	if _, genuine := keys.open(now, next); !genuine {
		t.Fatal("a challenge of the key before the current one is not read")
	}
	// A process that stood still for longer than two key lifetimes keeps no old key at all.
	now = now.Add(3 * challengeKeyLife)
	if _, _, err := keys.keys(now); err != nil || keys.previous != nil {
		t.Fatalf("an old key was kept past its use: %v", err)
	}
}
