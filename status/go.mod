module github.com/madsciencezone/busylight

go 1.16

require (
	go.bug.st/serial v1.3.5
	golang.org/x/net v0.0.0-20210226172049-e18ecbb05110
	golang.org/x/oauth2 v0.0.0-20210220000619-9bb904979d93
	google.golang.org/api v0.41.0
	internal/busylight v0.0.0
)

replace internal/busylight => ./internal/busylight
