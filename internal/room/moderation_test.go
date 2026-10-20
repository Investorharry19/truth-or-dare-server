package room

import "testing"

func TestContainsBannedWordIsCaseInsensitive(t *testing.T) {
	r := &Room{BannedWords: []string{"spoiler", " secret ", ""}}

	if !containsBannedWord(r, "This contains a SECRET") {
		t.Fatal("expected banned text to be detected")
	}
}

func TestContainsBannedWordAllowsTextWithoutBannedWords(t *testing.T) {
	r := &Room{BannedWords: []string{"spoiler", "secret"}}

	if containsBannedWord(r, "This message is fine") {
		t.Fatal("expected clean text to be allowed")
	}
}

func TestContainsBannedWordIgnoresEmptyBannedWords(t *testing.T) {
	r := &Room{BannedWords: []string{"", "   "}}

	if containsBannedWord(r, "Any message") {
		t.Fatal("expected empty banned words to be ignored")
	}
}
