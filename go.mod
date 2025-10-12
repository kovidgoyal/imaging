module github.com/kovidgoyal/imaging

go 1.24.0

require (
	github.com/google/go-cmp v0.7.0
	github.com/kovidgoyal/go-parallel v1.0.1
	github.com/mandykoh/prism v0.35.3
	github.com/rwcarlsen/goexif v0.0.0-20190401172101-9e8deecbddbd
	golang.org/x/image v0.31.0
)

replace github.com/mandykoh/prism => github.com/kovidgoyal/prism v0.0.0-20251012091921-e9749344d789

// replace github.com/mandykoh/prism => ../prism
