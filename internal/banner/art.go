package banner

// artRow is one line of the wordmark: the glyphs, and a colour key for the
// foreground and background of each cell.
type artRow struct {
	glyphs string
	fg     string
	bg     string
}

// The wordmark, one line of glyphs with a colour key for each.
//
// Each cell carries its own foreground and background, which is what gives
// the letters their carved edge. The keys are: L a lit parchment face, S the
// dry olive it falls away to, B the leaf green of the body, D the deeper
// green beneath it, and _ for nothing at all.
var art = []artRow{
	{`██▀▀▄   ▄▀▀▄  █▄▄  ▄▄█  ▄▄▄▄  █▄▄  ▄▄▀     `, `LLSSSSSSSSSSSSSLLSSLLSSSSLLLSSSLLSSLLSSSSSS`, `_______________SS__SS____SSS___SS__SS______`},
	{`██  █▄ ▄█  █▄  ██  ██  ▄█ ▀▀   ██  ██      `, `LLSSLLSLLSSLLSSLLSSLLSSLLSLLSSSLLSSLLSSSSSS`, `_D__DS_SD__DS__DD__DD__SD_SS___DD__DD______`},
	{`██  ▀▀ ██  ██  ██  ██  ██      ██  ██      `, `LLSSLSSLLSSLLSSLLSSLLSSLLSSSSSSLLSSLLSSSSSS`, `DD__S__DD__DD__DD__DD__DD______DD__DD______`},
	{`██▀▀▄  ██  ██  ██  ▒▒  ▓▓▐▀██  ██▄▄▄▄      `, `LLSSSSSLLSSLLSSLLSSLLSSLLSLLLSSLLSSSLSSSSSS`, `DD_____DD__DD__DD__DD__DD_SDD__DD___S______`},
	{`▓▓  ▓▄ ▓▓  ▓▓  ▓▓  ▓▓  ▓▓  ▄▄  ▓▓  ▓▓      `, `BBSSBBSBBSSBBSSBBSSBBSSBBSSSLSSBBSSBBSSSSSS`, `DD__DS_DD__DD__DD__DD__DD___S__DD__DD______`},
	{`██  █▀ ▀█  █▀  ▀█  █▀  ▀█  ▓▓  ██  ██      `, `BBSSBBSBBSSBBSSBBSSBBSSBBSSBBSSBBSSBBSSSSSS`, `_D__DD_DD__DD__DD__DD__DD__DD__DD__DD______`},
	{`██▄▄▀   ▀▄▄▀    ▀▄▄▀    ▀▀▀ ▀ ▀▀▀  ▀▀▀     `, `BBBDDSSSDDDDSSSSDDDDSSSSDBBBDSBBBSSDBBSSSSS`, `_________________________DDD__DDD___DD_____`},
}
