module github.com/madsciencezone/busylight

go 1.20

require (
	github.com/MadScienceZone/atk v1.2.2
	golang.org/x/net v0.1.0
	golang.org/x/oauth2 v0.0.0-20210220000619-9bb904979d93
	google.golang.org/api v0.41.0
	internal/busylight v0.0.0
)

require (
	cloud.google.com/go v0.78.0 // indirect
	github.com/MadScienceZone/go-gma/v5 v5.4.0 // indirect
	github.com/creack/goselect v0.1.2 // indirect
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/googleapis/gax-go/v2 v2.0.5 // indirect
	go.bug.st/serial v1.3.5 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/sys v0.1.0 // indirect
	golang.org/x/text v0.4.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20210303154014-9728d6b83eeb // indirect
	google.golang.org/grpc v1.49.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

replace internal/busylight => ./internal/busylight

replace github.com/MadScienceZone/atk => ../../atk
