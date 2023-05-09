#!/usr/bin/env wish8.6
#
# Control busylight from GUI
#
package require json 1.3.4
package require json::write

set config_file_name [file normalize [file join ~ .busylight config.json]]
set activity_file_name [file normalize [file join ~ .busylight activities.json]]
if {[catch {set f [open $config_file_name]} err]} {
	puts stderr "unable to open $config_file_name: $err"
	exit 1
}
set config_data [::json::json2dict [read $f]]
close $f

#
# Colors	stack of light colors
# StatusLights	dict of status->command
# 


set dev_state [dict create \
	PortOpen false \
	Port     {} \
	PortName {} \
	IsLightOn {} \
	Flasher [dict create IsOn false Sequence {}] \
	Strober [dict create IsOn false Sequence {}] \
]


proc load_activities {filename} {
	if {[catch {set f [open $filename]} err]} {
		puts stderr "unable to open activities file: $err"
		exit 1
	}
	set d [::json::json2dict [read $f]]
	close $f
	return $d
}

set activities [load_activities $activity_file_name]

proc save_activities {filename data} {
	if {[catch {set f [open $filename w]} err]} {
		puts stderr "unable to save activities: $err"
		return
	}
	::json::write indented 1
	::json::write aligned 1
	set l {}
	foreach activity $data {
		set s {}
		foreach stat [dict get $activity Status] {
			lappend s [::json::write string $stat]
		}
		lappend l [::json::write object \
			Name [::json::write string [dict get $activity Name]] \
			Status [::json::write array {*}$s] \
			Elapsed [dict get $activity Elapsed] \
		]
	}
	puts $f [::json::write array {*}$l]
	close $f
}

#
# Update the device status by running busylight -query
# Raw response data: [76 48 48 48 48 49 48 48 70 49 88 83 48 88]
#
# response string is
#                  sequence
#            index  __|___
#                | /      \
#                n@xxxxx...         n@xxxxx...
#    L011100...F0X                S0X             \n
#     \______/ :| \              :             :
#        |     :|  if no sequence:             :
#      0=off  0=off              :             :
#      1=on   1=on               :             :
#    Each LED timer              :             :
#              |_________________|_____________|
#                   flasher         strober
#
proc _update_status {statevar} {
	upvar $statevar s

	if [catch {
		if [regexp {Raw response data: \[(.*?)\]} [exec busylight -query] _ raw_data] {
		} else {
			error "unable to get light status"
		}
	} err] {
		error "error getting light status: $err"
	}

	set state {}
	set fseq {}
	set sseq {}
	dict set s IsLightOn {}
	foreach byte $raw_data {
		switch -exact -- $byte {
			76 { set state lights }
			70 { set state flasher }
			83 { set state strober }
			88 { set state {} }
			64 { append state ,sequence }
			48 - 49 { 
				if {$byte eq 48} {
					set value false
					set svalue 0
				} else {
					set value true
					set svalue 1
				}

				switch -exact -- $state {
					fpos,sequence {lappend fseq $svalue}
					spos,sequence {lappend sseq $svalue}
					flasher       {dict set s Flasher IsOn $value; set state fpos}
					strober       {dict set s Strober IsOn $value; set state spos}
					fpos          {}
					spos          {}
					lights        {dict lappend s IsLightOn $value}
					default {
						error "unexpected byte $byte w/o state ($raw_data)"
					}
				}
			}
			50 - 51 - 52 - 53 - 54 {
				switch -exact -- $state {
					fpos,sequence {lappend fseq [format %c $byte]}
					spos,sequence {lappend sseq [format %c $byte]}
					default {
						error "unexpected byte $byte outside sequence ($raw_data)"
					}
				}
			}
			
			default {
				error "unexpected byte $byte in device status ($raw_data)"
			}
		}
	}
	dict set s Flasher Sequence $fseq
	dict set s Strober Sequence $sseq
}

proc signal_error {message} {
	tk_messageBox -type ok -icon error -title "Busylight Error" -message $message -parent .
	# TODO set light pattern
}

foreach {name server external internal} {
	{server mute}   true {-mute}   {}
	{server open}   true {-open}   {}
	{server cal}    true {-cal}    {}
	{server wake}   true {-wake}   {}
	{server reload} true {-reload} {}
	{server zzz}    true {-zzz}    {}
	{refresh}       false {} {refresh_all $config_data dev_state}
} {
	array set std_commands [list \
		$name,server   $server \
		$name,external $external \
		$name,internal $internal\
	]
	lappend std_commands(names) $name
}

proc refresh_all {config_data dev_state} {
	upvar $dev_state state
	_update_status state
	update_lights $config_data state
	server_status_check $config_data
}

proc _set_lights {dev_state} {
	upvar $dev_state state
	global light

	set i 0
	foreach light_state [dict get $state IsLightOn] {
		if {[info exists light(slot,$i)]} {
			set cc $light(slot,$i)
			set w $light($i,widget)
			if {$light_state} {
				$w configure -background $light($cc,on)
			} else {
				$w configure -background $light($cc,off)
			}
		}
		incr i
	}
}

set cur_activity {}
set elapsed_time 0
set timer_id {}
proc stop_timer {} {
	global cur_activity timer_id activities elapsed_time activity_file_name
	if {$timer_id ne {}} {
		after cancel $timer_id
		set timer_id {}
	}
	if {$cur_activity ne {}} {
		set d [lindex $activities $cur_activity]
		dict set d Elapsed $elapsed_time
		set activities [lreplace $activities $cur_activity $cur_activity $d]
		set cur_activity {}
	}
	_update_time_displays
	save_activities $activity_file_name $activities
}

proc start_activity {name} {
	global config_data dev_state activities cur_activity timer_id elapsed_time
	stop_timer
	for {set i 0} {$i < [llength $activities]} {incr i} {
		if {[dict get [lindex $activities $i] Name] eq $name} {
			set cur_activity $i
			set elapsed_time [dict get [lindex $activities $i] Elapsed]
			set timer_id [after 60000 _advance_timer]
			_update_time_displays
			update
			foreach status [dict get [lindex $activities $i] Status] {
				busylight {*}$status
			}
			return
		}
	}
}

proc _advance_timer {} {
	global cur_activity timer_id elapsed_time activities activity_file_name
	if {$timer_id eq {} || $cur_activity eq {}} {
		return
	}
	incr elapsed_time
	set d [lindex $activities $cur_activity]
	dict set d Elapsed $elapsed_time
	set activities [lreplace $activities $cur_activity $cur_activity $d]
	save_activities $activity_file_name $activities
	_update_time_displays
	set timer_id [after 60000 _advance_timer]
}

proc _update_time_displays {} {
	global light cur_activity timer_id elapsed_time activities
	for {set i 0} {$i < [llength $activities]} {incr i} {
		set j [expr $i + $light(act,offset)]
		if {$i == $cur_activity} {
			$light($j,act,widget) configure -foreground green \
				-text [format "%s %d:%02d" $light($j,act,name) \
					[expr $elapsed_time / 60] \
					[expr $elapsed_time % 60]]
		} else {
			$light($j,act,widget) configure -foreground blue \
				-text [format "%s %d:%02d" $light($j,act,name) \
					[expr [dict get [lindex $activities $i] Elapsed] / 60] \
					[expr [dict get [lindex $activities $i] Elapsed] % 60] \
				]
		}
	}
}

proc do_busylight {args} {
	stop_timer
	busylight {*}$args
}

proc busylight {args} {
	global config_data dev_state
	exec busylight {*}$args
	refresh_all $config_data dev_state
}

proc update_lights {config_data dev_state} {
	upvar $dev_state state
	global light

	_set_lights state

	if {[set s [dict get $state Flasher Sequence]] ne {}} {
		set_flasher $s 200
	} else {
		clear_flasher
	}

	if {[set s [dict get $state Strober Sequence]] ne {}} {
		set_strober $s 50 2000
	} else {
		clear_strober
	}
}

set flasher_data(id) {}
set flasher_data(sid) {}
proc set_flasher {sequence ontime} {
	global flasher_data
	if {$flasher_data(id) ne {}} {
		clear_flasher
	}
	set flasher_data(id) [after 100 _flash_on 0 [list $sequence] $ontime dev_state -start]
}

proc clear_flasher {} {
	global flasher_data
	if {$flasher_data(id) ne {}} {
		after cancel $flasher_data(id)
		set flasher_data(id) {}
	}
}

proc clear_strober {} {
	global flasher_data
	if {$flasher_data(sid) ne {}} {
		after cancel $flasher_data(sid)
		set flasher_data(sid) {}
	}
}

proc set_strober {sequence ontime offtime} {
	global flasher_data
	if {$flasher_data(sid) ne {}} {
		clear_strober
	}
	set flasher_data(sid) [after $offtime _strobe_on 0 [list $sequence] $ontime $offtime dev_state -start]
}

proc _strobe_on {pos sequence ontime offtime statevar args} {
	global $statevar
	global flasher_data

	if {$flasher_data(sid) ne {} || $args eq {-start}} {
		set lpos [lindex $sequence $pos]
		dict set $statevar IsLightOn [lreplace [dict get [set $statevar] IsLightOn] $lpos $lpos true]
		_set_lights $statevar
		set flasher_data(sid) [after $ontime _strobe_off $pos [list $sequence] $ontime $offtime $statevar]
	}
}

proc _strobe_off {pos sequence ontime offtime statevar} {
	global $statevar
	global flasher_data
	if {$flasher_data(sid) ne {}} {
		set lpos [lindex $sequence $pos]
		dict set $statevar IsLightOn [lreplace [dict get [set $statevar] IsLightOn] $lpos $lpos false]
		_set_lights $statevar
		set pos [expr ($pos+1) % [llength $sequence]]
		set flasher_data(sid) [after $offtime _strobe_on $pos [list $sequence] $ontime $offtime $statevar]
	}
}

proc _flash_on {pos sequence ontime statevar args} {
	global light flasher_data $statevar
	
	if {$flasher_data(id) eq {} && $args ne {-start}} {
		return
	}

	if {$pos >= [llength $sequence]} {
		set pos 0
	}

	dict set $statevar IsLightOn {}
	for {set i 0} {$i < 7} {incr i} {
		if {[info exists light(slot,$i)]} {
			if {[lindex $sequence $pos] == $i} {
				dict lappend $statevar IsLightOn true
			} else {
				dict lappend $statevar IsLightOn false
			}
		}
	}
	_set_lights $statevar
	set flasher_data(id) [after $ontime _flash_on [expr $pos+1] [list $sequence] $ontime $statevar]
}

set daemon_pid {}
proc server_status_check {config_data} {
	global daemon_pid
	global std_commands
	get_server_pid $config_data

	if {$daemon_pid eq {}} {
		foreach {_ w} [array get std_commands *,widget] {
			$w configure -state disabled
		}
	} else {
		foreach {_ w} [array get std_commands *,widget] {
			$w configure -state normal
		}
	}
}

proc get_server_pid {config_data} {
	global daemon_pid

	if {[catch {set f [open [file normalize [dict get $config_data PidFile]]]} err]} {
		set daemon_pid {}
		puts "Unable to read pid file: $err"
		return
	}
	gets $f daemon_pid
	close $f
	if {[catch {set process_info [exec ps $daemon_pid]}]} {
		puts "Process $daemon_pid does not seem to exist; assuming daemon is not running"
		set daemon_pid {}
		return
	}
	if {[string first busylightd $process_info] < 0} {
		puts "Process $daemon_pid does not seem to be busylightd; assuming daemon is not running"
		set daemon_pid {}
	} 
}

proc send_to_server {fmt} {}

pack [frame .l] -side top -expand 1 -fill both
pack [frame .b] -side top -expand 1 -fill both

set level -1
foreach color_code [split [dict get $config_data Colors] {}] {
	set light(slot,[incr level]) $color_code
	set light($level,widget) .l.$level
	set light($color_code,on)  [dict get $config_data ColorValues $color_code]
	set light($color_code,off) [::tk::Darken [dict get $config_data ColorValues $color_code] 25]
	pack [label .l.$level -text "                        " -background $light($color_code,off) -highlightbackground $light($color_code,on) -highlightthickness 2] -side top -pady 2
}

set i 0
dict for {status cmd} [dict get $config_data StatusLights] {
	global i
	if {$status ne {start} && $status ne {stop}} {
		pack [button .b.$i -text $status -command "busylight -status $status"] -side top -fill x
	}
	incr i
}
set light(act,offset) $i
foreach activity $activities {
	set light($i,act,widget) .b.$i
	set light($i,act,name) [dict get $activity Name]
	pack [button .b.$i -text [dict get $activity Name] -command "start_activity [list [dict get $activity Name]]" -foreground blue] -fill both
	incr i
}
pack [button .b.$i -text {(stop activity)} -command "stop_timer" -foreground blue] -fill both
incr i

foreach name $std_commands(names) {
	if $std_commands($name,server) {
		pack [button .b.$i -text $name -command "busylight $std_commands($name,external)" -foreground red] -side top -fill x
	} else {
		pack [button .b.$i -text $name -command $std_commands($name,internal) -foreground red] -side top -fill x
	}

	set std_commands($name,widget) .b.$i
	incr i
}

refresh_all $config_data dev_state
_update_time_displays

proc _periodic_refresh {} {
	global config_data dev_state
	refresh_all $config_data dev_state
	after 300000 _periodic_refresh
}

after 300000 _periodic_refresh
