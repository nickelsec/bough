package segment

import "strings"

// stopwords are words too common to say anything about what a prompt is about.
//
// The list leans heavily on the way people talk to coding agents, which is full
// of politeness and instruction verbs that appear in every prompt regardless of
// subject.
var stopwords = map[string]bool{}

func init() {
	const list = `the a an and or but if then this that these those is are was
were be been being to of in on for with at by from as it its i you we they my
our your their me him her them so just like about into over under can could
should would will shall do does did have has had not no yes get got make made
want need see look know think say said very really quite too also more most
some any all one two new now here there when where how why what which who whom
whose please thanks thank okay ok sure cool nice good great yeah yep going go
lets let put set try use give take add fix change remove move show tell keep
still back out up down off again else same other than only even much many

file files code line lines function functions run running works working
work done finish finished create created update updated`

	for _, w := range strings.Fields(list) {
		stopwords[w] = true
	}
}

// wordsOf reduces a prompt to the words worth comparing.
//
// Anything shorter than three letters, and anything on the stopword list, is
// dropped. What is left is mostly the nouns naming what the user was working
// on, which is the only part that distinguishes one prompt from another.
func wordsOf(text string) []string {
	var out []string
	seen := map[string]bool{}

	for _, field := range strings.FieldsFunc(strings.ToLower(text), notWordRune) {
		if len(field) < 3 || stopwords[field] || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}

// notWordRune splits on anything that is not part of an identifier, so that
// file names and symbols survive as single tokens.
func notWordRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return false
	case r >= '0' && r <= '9':
		return false
	case r == '_' || r == '-' || r == '.':
		return false
	default:
		return true
	}
}
