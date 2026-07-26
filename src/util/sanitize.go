package util

import (
	"regexp"
	"strings"
)

var (
	filenameRe = regexp.MustCompile(`[^\p{L}\d._,\-]+`)
	alnumRe    = regexp.MustCompile(`[^\p{L}\d]+`)
	// Trailing "(feat. X)", "[ft. X]", "(featuring X)". Only a trailing annotation is
	// stripped, so a title that genuinely reads "Duet with Death" keeps its words.
	//
	// "with" is deliberately not a credit marker here even though some services use
	// "(with X)": titles beginning a parenthetical with it are far more common than
	// the credit form, and treating it as one turns "Baby (With You)" into "Baby",
	// which then collides with any track actually called "Baby".
	featTailRe = regexp.MustCompile(`(?i)\s*[\(\[\{]\s*(feat\.?|featuring|ft\.?)\s[^\)\]\}]*[\)\]\}]\s*$`)
	// Trailing "- 2011 Remaster" / "– 1999 Remastered".
	remasterTailRe = regexp.MustCompile(`(?i)\s*[-–—]\s*\d{4}\s*remaster(ed)?\s*$`)
)

// FilenameSafe replaces characters unsafe for filenames with '_'
func FilenameSafe(s string) string {
	return filenameRe.ReplaceAllString(s, "_")
}

// AlnumOnly removes everything except letters and digits
func AlnumOnly(s string) string {
	return alnumRe.ReplaceAllString(s, "")
}

// ContainsFold reports whether substr is within s, ignoring case.
func ContainsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// CleanSearchTitle strips a trailing featured-artist annotation and remaster suffix
// while keeping the title readable, for use as a search API query. Music servers
// index the plain title, so searching for "Song (feat. Guest)" often returns nothing.
func CleanSearchTitle(s string) string {
	return strings.TrimSpace(stripTitleTail(s))
}

// NormalizeTitle reduces a title to lowercase letters and digits, after dropping the
// same trailing annotations, so two spellings of one title compare equal.
func NormalizeTitle(s string) string {
	return AlnumOnly(strings.ToLower(stripTitleTail(s)))
}

// stripTitleTail removes trailing annotations until none are left. A single pass is
// not enough: both patterns only match at the end of the string, so "Song (feat. X)
// - 2011 Remaster" needs the remaster suffix gone before the credit is at the end.
func stripTitleTail(s string) string {
	for {
		stripped := remasterTailRe.ReplaceAllString(featTailRe.ReplaceAllString(s, ""), "")
		if stripped == s {
			return s
		}
		s = stripped
	}
}
