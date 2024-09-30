# ARCHIVED
As of September, 2024, this project is archived. Another project which has long been in the works,
[Readerboard](https://github.com/MadScienceZone/readerboard), defines hardware, firmware, and software
to drive LED matrix displays which happen to also include an 8-LED status indicator display which exactly
mirrors the operation of a Busylight unit. As such, a Readerboard unit is a proper superset of a Busylight
unit, so it made sense to merge the Busylight project into it.

Going forward, the firmware for both Busylights and Readerboards is compiled from a common source code base,
which we consider preferable to maintaining two separate but similar code bases.  Over on the Readerboard
project there are also a number of hardware improvements over the older Busylight units archived here.

# busylight
DIY computer "do not disturb" indicator

This was a weekend project to see if I could take the stuff in my junk drawer and implement a simple light that would show my family members if it was safe to interrupt me without accidentally having their voices or faces appear in a meeting (this is during the work-at-home pandemic restrictions).

## Operation
The light is placed in a convenient and visible location, and displays lights to indicate my current "busy" state.
This version of the Busylight project allows for up to 7 lights of arbitrary colors.

My original used these colors:
* Green: interrupt at will.
* Yellow: interrupt if important (this is shown if I happen to have anything scheduled on my Google calendars, such as a meeting or just some time blocked out to focus on a project)
* Red: in a meeting (this is shown if I'm actually connected to a video conference meeting)
* Flashing red: in a meeting, and my microphone is unmuted
* Flashing blue/red: urgent status
* Strobing green: (along with other lights) low-priority meeting, be aware I'm on camera but interruptions are ok


## Hardware
For the light itself, I grabbed a spare Arduino Pro Micro board, a ULN2003A darlington transistor array chip, and a bunch of LEDs and resistors and threw them together.
I chose resistor values to get a suitable brightness from the LEDs (which varied by color and the voltage ratings of the LEDs; your mileage may vary) without overloading
the USB supply current limits.

The Arduino board and LEDs are powered by the USB port from the host PC, as well as using that connection for the host to send serial commands to control the lights.

To make this easier, I created a shield PCB to neatly interface the high current driver chip to an 8-bin DIN connector for the light tree.

## Software
On the PC, the normal mode of operation is to run the `busylightd` daemon. This monitors a set of Google calendars and reports busy/free times with the green and yellow
lights. It also responds to external signals from other processes which can inform it of other status changes.

In my case, I used [Hammerspoon](https://www.hammerspoon.org) which provides extensible automation capabilities for MacOS systems, including a very handy plugin that detects
when you join or leave a Zoom call, and tracks the state of the mute controls while in the meeting. I just configured that to send the appropriate signals to the
*busylightd* process when those statuses changed.

The upshot of that is that I can leave this running, put busy time on my calendars, join Zoom calls, and so forth, while the light indicator automatically displays
the appropriate colors.

### API Access
To get access to the Google calendar API, you'll need to register with Google and get an API key. If I were distributing this as a pre-made app, I'd include my API key
with the distribution, but as a DIY project, if you make one of these based on my design, you're essentially creating your own app anyway so it makes sense to get a
separate API key for yours.

### Documentation
There's a *busylight(1)* manual page included which explains the setup and operation of the software.

A schematic of how I wired up the hardware is also included, although it's fairly trivial, and can be adjusted to suit your needs.

### GUI Tool
A simple tcl/tk script is supplied which provides a convenient front-end to the *busylight* program, with the addition of task time tracking.

A *blight(1)* manual page is provided to explain it.

# Release notes
See the `CHANGELOG.md` file.
