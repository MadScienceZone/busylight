## Version 1.10.0
### Blight changes
 * Added "Clear times" button and improved support for tracking activity times.
 * Introduced 1 second delay before updating display to allow time for the light device to complete what it was supposed to do. 

## Version 1.9.1
Improved blight script, also added `ColorValues` field to `config.json`.

## Version 1.9.0
Added a simple GUI front-end which provides buttons to change status on the lights as well
as simple time tracking of activities.

## Version 1.8.2
Added an output line to the `-query` option to indicate whether the daemon is
currently running or not.

## Version 1.8.1
Added the ability to name the light colors for a more human-friendly output
from the `-query` option.

## Version 1.8
Added a `-query` option to `busylight` and corresponding firmware support to allow
the user to ask the hardware which lights are on and what sequences are playing.

## Version 1.7
Added a `-list` option to `busylight` which lists out all the defined status codes
(from the configuration file) that the user can specify with the `-status` option.

## Version 1.6.1
Made a slight change to the order in which `busylight` sends commands to the daemon,
so that `-wake` happens first in the sequence and `-zzz` happens last.

## Version 1.6
Colors are now arbitrary and up to 7 lights are supported. New shield PCB introduced.
New protocol introduced to support more arbitrary light patterns.

Removed superfluous `busylight-standalone` command. Now both `busylight` and `busylightd` can
directly update the device since as of this version neither leaves the I/O
port open all the time. In case of a collision, the programs will wait for the other to
let go of the port before proceeding.

## Version 1.5
Adds a low-priority indicator to be added in addition to the other
markers so you can say things like "I'm in a video call but it's not
so important that I can't be interrupted. Just understand I'm on camera."

Also re-added the 1.4 changes to the repo since apparently they didn't get
committed correctly before.

Also also becoming very apparent that using signals to notify the daemon
of changes in state was a bad idea (given the limited availability of
signals that are safe to co-opt for this usage), so a future version will 
likely introduce a new mechanism for that.

## Version 1.4
Allows a regular expression for the device name now, so that the daemon
can search for whatever name the OS randomly chose for the hardware device.

## Version 1.3
The daemon now closes the serial port when put in inactive state and
re-opens it when going active again. It also reloads its configuration
file at that time, making it possible to change the serial device in case
the system gave it a new dynamic device name during the time the daemon
was sleeping.

This also allows other configuration changes to be made  without restarting
the daemon (just set it to inactive state and back to active again).

Refactored some code to clean it up a little. 

## Version 1.2
The 1.2 release includes the ability to ignore long-running appointments on selected calendars, to avoid
signalling that the user is "busy" because of an all-day event on a group calendar.

This made it necessary to change the `config.json` file's list of calendars to be monitored. Users of the 1.1 version will need to 
update this configuration file after upgrading to 1.2.
