module github.com/athenianco/circleci-export

go 1.18

require (
	github.com/rs/zerolog v1.26.1
	github.com/schollz/progressbar/v3 v3.12.1
)

require (
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/rivo/uniseg v0.4.2 // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/term v0.1.0 // indirect
)

replace (
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20220926161630-eccd6366d1be
	golang.org/x/net => golang.org/x/net v0.0.0-20220927171203-f486391704dc
	golang.org/x/text => golang.org/x/text v0.3.8
)
