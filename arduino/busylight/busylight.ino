// Change the following PER DEVICE before flashing.
#define THIS_DEVICE_COLOR_MAP "BRrYG"
#define SERIAL_VERSION_STAMP "V1.0.2$R2.0.0$SB001$"
//                             \___/  \___/  \__/
//                               |      |      |
//                  Hardware version    |      |
//                         Firmware version    |
//                                 Serial number
//
// serial numbers B000-B299 reserved for author's use

/*
** Steve Willoughby <steve@madscience.zone>
** Licensing: BSD 3-clause open-source license
**
** Simple busylight indicator, controlled via USB serial commands.
** To use, open the USB device as a serial port and send commands to it. 
**
** The physical LED tree is stacked like so:
** The colors are suggested but they can be arbitrary.
**
**                    BLUE    ==================	#0
**                                    ||
**                    RED 2   ==================	#1
**                                    ||
**                    RED 1   ==================	#2
**                                    ||
**                    YELLOW  ==================	#3
**                                    ||
**                    GREEN   ==================	#4
**                                    ||
**                            ==================	#5
**                                    ||
**                            ==================	#6
**                                    ||
**                                    ||
**                                    ||
**                                    ||
**                                    ||
**
** PROTOCOL V2
** Legacy commands from V1 no longer supported.
**
** Commands received via USB. The V2 protocol is designed to be
** compatible with the author's readerboard project, which implies
** in the future there could be support for RS-485 usage as well,
** but that's not implemented here at this time.
**
** All commands are terminated with a ^D ($04) byte. If an error
** occurs, all data are ignored until a ^D is received. Extra data
** after a command (but before the terminating ^D) is ignored.
**
** * <L0> <L1> ... <Ln-1> $     strobe LEDs
** = __ <spd>                   set baud rate to <spd>
** ?                            query LED status
** F <L0> <L1> ... <Ln-1> $     flash lights in sequence
** Q                            query device status
** S <L0>                       light single LED steady
** X                            turn off LEDs
**
** <Ln> - ASCII digit '0'..'6'.
** <spd> - 0=300 1=600 2=1200 3=2400 4=4800 5=9600* 6=14.4k 7=19.2k 8=28.8k 9=31.25k A=38.4k B=57.6k C=115.2k
** *default
**
** Set this for each individual unit with the hardware version and firmware version.
** Also set the unit's serial number in place of the XXXXX.  Serial numbers 000-299 are
** reserved for the author's use.
**
** Each of these fields are variable width, do not necessarily have leading zeroes,
** and the version numbers may be any string conforming to semantic versioning 2.0.0
** (see semver.org).
**
**
** Implementation Notes
** USB
**     Commands received via USB must be terminated by a ^D. If an error is encountered,
**     the interpreter will ignore data until a ^D is received before starting to interpret
**     anything further.
**
** State Machine
**     -> ERR       signal error condition and go to ERROR state
**     -> END       go to END state
**     +            change transition and then re-examine current input byte
**                          
**                    
**                    
**                    
**  _____     ^D        ________ 
** |ERROR|----------->||Idle    ||
** |_____|<-----------||________||
**   ^ |       *            |
**   |_|*                   |                              _
//          ?/Q             |            ____             | |*
//  END <-------------------+ '*'   ____|_   | led       _V_|_  ^D
//                          +----->|Strobe|<-+      $   |END  |-----------> Idle
//   _____    =             |      |______|------------>|_____|
//  |Set  |<----------------+ 
//  |_____|                 |
//    | *                   |
//   _V_____                |
//  |SetUspd|               |
//  |_______|               |
//    | speed               |
//    V                     |           ____
//   END                    |  F    ___|_   |led
//                          +----->|Flash|<-+      $
//                          |      |_____|----------> END
//                          |  S    ________ led
//                          +----->|LightSet|----------> END
//                          |      |________| 
//                          |  X
//                          +-----> END
**
** Responses:
** ?
**     L <L0> <l1> ... <L6> $ F <flasher-status> $ S <strober-status> $ \n
**        <*-status> ::= <running?> _             (no sequence)
**                     | <running?> <pos> @ (<colorcode>|<digit>)*
**        <running?> ::= R=running | S=stopped
**        <pos> ::= encoded position value 0-63 as 0-9:;<=>?@A-Z[\]^_`a-o (numeric value+48)
**        <Ln> ::= <colorcode>|<digit>|_ (off)|? (out of range)
**
** Q
**     Q B = _ <speed> _ _ $ V <hwversion> $ R <romversion> $ S <serialno> $ \n
**
*/

#include <TimerEvent.h>
#include <EEPROM.h>

#define COUNTOF(X) (sizeof(X)/sizeof(X[0]))

//
// EEPROM locations
// $00 0x4B
// $01 baud rate code
//
#define EE_ADDR_SENTINEL  (0x00)
#define EE_ADDR_USB_SPEED (0x01)
#define EE_VALUE_SENTINEL (0xb1)
#define EE_DEFAULT_SPEED  ('5')

//
// Digital output pin numbers for the lights
// (a high output turns on the LEDs)
//                       BL R2 R1  Y  G
const int tree_port[] = {10, 7, 6, 8, 9, 14, 16};
//                       #0 #1 #2 #3 #4  #5  #6

//
// These LightBlinkers handle our flashing and strobing.
// In each case, they sequence through one or more lights
// at each flash.
// 
#define MAX_SEQUENCE    (64)
#define SEQUENCE_ERROR (254)	// error in sequence codes
#define SEQUENCE_OFF   (255)    // spot in sequence where LEDs are off
class LightBlinker {
	unsigned int on_period;	  // in mS 
	unsigned int off_period;  // in mS
	bool         cur_state;	  // are we on?
	unsigned int cur_index;   // index into sequence
	unsigned int sequence_length;
	byte         sequence[MAX_SEQUENCE];
	TimerEvent   timer;

public:
	LightBlinker(unsigned int on, unsigned int off, void (*callback)(void));
	void update(void);
	void stop(void);
	void append(int);
	int  length(void);
	void advance(void);
	void start(void);
	void report_state(void);
};

LightBlinker::LightBlinker(unsigned int on, unsigned int off, void (*callback)(void))
{
	timer.set(0, callback);
	timer.disable();
	cur_state = false;
	cur_index = 0;
	sequence_length = 0;
	on_period = on;
	off_period = off;
}

int LightBlinker::length(void)
{
	return sequence_length;
}

void LightBlinker::append(int v)
{
	if (sequence_length < MAX_SEQUENCE) {
		sequence[sequence_length++] = v;
	}
}

void LightBlinker::report_state(void)
{
	// report state as cur_state (R or S) then "X" (off) or "<index>@<sequence>" + "$"
	int i = 0;
	Serial.write(cur_state ? 'R' : 'S');
	if (sequence_length > 0) {
		Serial.write(cur_index + '0');
		Serial.write('@');
		for (i = 0; i < sequence_length; i++) {
			if (sequence[i] == SEQUENCE_OFF) {
				Serial.write('_');
			} else if (i >= COUNTOF(tree_port)) {
				Serial.write('?');
			} else {
				if (i < strlen(THIS_DEVICE_COLOR_MAP)) {
					Serial.write(THIS_DEVICE_COLOR_MAP[i]);
				} else {
					Serial.write(sequence[i] + '0');
				}
			}
		}
	} else {
		Serial.write('X');
	}
	Serial.write('$');
}

void LightBlinker::advance(void)
{
	// If we have a sequence of one item, we will just flash that one on and off
	// If we have no off_period, it's the same as the on_period
	if (sequence_length < 2) {
		if (cur_state) {
			if (sequence[0] != SEQUENCE_OFF && sequence[0] < COUNTOF(tree_port)) {
				digitalWrite(tree_port[sequence[0]], LOW);
			}
			cur_state = false;
			if (off_period > 0)
				timer.setPeriod(off_period);
		} else {
			if (sequence[0] != SEQUENCE_OFF && sequence[0] < COUNTOF(tree_port)) {
				digitalWrite(tree_port[sequence[0]], HIGH);
			}
			cur_state = true;
			if (off_period > 0)
				timer.setPeriod(on_period);
		}
		return;
	}
	
	// Otherwise we just change to the next light in the sequence
	// If we have no off_period, just switch to the next one. Otherwise,
	// only advance on the "on" transition, so we quickly flash each in
	// turn.
	if (sequence_length > MAX_SEQUENCE)
		sequence_length = MAX_SEQUENCE;

	if (off_period == 0) {
		cur_state = true;
		if (sequence[cur_index] != SEQUENCE_OFF && sequence[cur_index] < COUNTOF(tree_port)) {
			digitalWrite(tree_port[sequence[cur_index]], LOW);
		}
		cur_index = (cur_index + 1) % sequence_length;
		if (sequence[cur_index] != SEQUENCE_OFF && sequence[cur_index] < COUNTOF(tree_port)) {
			digitalWrite(tree_port[sequence[cur_index]], HIGH);
		}
	}
	else {
		if (cur_state) {
			if (sequence[cur_index] != SEQUENCE_OFF && sequence[cur_index] < COUNTOF(tree_port)) {
				digitalWrite(tree_port[sequence[cur_index]], LOW);
			}
			timer.setPeriod(off_period);
			cur_state = false;
		}
		else {
			cur_index = (cur_index + 1) % sequence_length;
			if (sequence[cur_index] != SEQUENCE_OFF && sequence[cur_index] < COUNTOF(tree_port)) {
				digitalWrite(tree_port[sequence[cur_index]], HIGH);
			}
			timer.setPeriod(on_period);
			cur_state = true;
		}
	}
}

void LightBlinker::update(void)
{
	timer.update();
}

void LightBlinker::stop(void)
{
	timer.disable();
	sequence_length = 0;
}

void LightBlinker::start(void)
{
	if (sequence_length > 0) {
		cur_index = 0;
		cur_state = true;
		if (sequence[0] != SEQUENCE_OFF && sequence[0] < COUNTOF(tree_port)) {
			digitalWrite(tree_port[sequence[0]], HIGH);
		}
		timer.reset();
		timer.setPeriod(on_period);
		timer.enable();
	}
	else {
		stop();
	}
}

const int CSM_BUFSIZE = 64;
class CommandStateMachine {
private:
	enum StateCode {
		IdleState,
		ErrorState,
		FlashState,
		LightSetState,
		StrobeState,
		SetState,
		SetUSpeedState,
		EndState,
	} state;
	byte LEDset;
	byte command_in_progress;
	byte buffer[CSM_BUFSIZE];
	byte buffer_idx;
public:
	void accept(int inputchar);
	bool accept_led_name(int inputchar);
	bool append_buffer(byte value);
	void begin(void);
	void end_cmd(void);
	void error(void);
	void reset(void);
};	

void CommandStateMachine::error(void) {
	digitalWrite(tree_port[0], HIGH);
	state = ErrorState;
}

void CommandStateMachine::end_cmd(void) {
	state = EndState;
}

void flash_lights(void);
void strobe_lights(void);
LightBlinker flasher(200, 0, flash_lights);
LightBlinker strober(50, 2000, strobe_lights);
CommandStateMachine csm;

void setup() {
	int i=0;
	//
	// digital output mode setting
	//
	for (i = 0; i < COUNTOF(tree_port); i++) {
		pinMode(tree_port[i], OUTPUT);
	}
	//
	// Cycle through all the lights at power-on
	// to test that they all work
	//
	for (i = 0; i < COUNTOF(tree_port); i++) {
		digitalWrite(tree_port[i], HIGH);
		delay(200);
		all_off(true);
	}

	default_eeprom_settings();
	flasher.stop();
	strober.stop();
	digitalWrite(tree_port[0], HIGH);
	start_usb_serial();
	digitalWrite(tree_port[0], LOW);
	csm.begin();
}

void default_eeprom_settings(void) {
	if (EEPROM.read(EE_ADDR_SENTINEL) != EE_VALUE_SENTINEL) {
		// apparently unset; set to "factory defaults"
		EEPROM.write(EE_ADDR_USB_SPEED, EE_DEFAULT_SPEED);
		EEPROM.write(EE_ADDR_SENTINEL, EE_VALUE_SENTINEL);
		return;
	}

	int speed = EEPROM.read(EE_ADDR_USB_SPEED);
	if (!((speed >= '0' && speed <= '9') 
	|| (speed >= 'a' && speed <= 'c') 
	|| (speed >= 'A' && speed <= 'C'))) {
		// baud rate setting invalid; return to default
		EEPROM.write(EE_ADDR_USB_SPEED, EE_DEFAULT_SPEED);
	}
}

void start_usb_serial(void) {
	long speed = 9600;

	switch (EEPROM.read(EE_ADDR_USB_SPEED)) {
		case '0': speed =    300; break;
		case '1': speed =    600; break;
		case '2': speed =   1200; break;
		case '3': speed =   2400; break;
		case '4': speed =   4800; break;
		case '5': speed =   9600; break;
		case '6': speed =  14400; break;
		case '7': speed =  19200; break;
		case '8': speed =  28800; break;
		case '9': speed =  31250; break;
		case 'a':
		case 'A': speed =  38400; break;
		case 'b':
		case 'B': speed =  57600; break;
		case 'c':
		case 'C': speed = 115200; break;
	}

	Serial.begin(speed);
	while (!Serial);
}

//
// all_off(): reset device to all off, no flashing
//
void all_off(bool reset_state) {
	if (reset_state) {
		flasher.stop();
		strober.stop();
	}
	for (int i=0; i < COUNTOF(tree_port); i++) {
		digitalWrite(tree_port[i], LOW);
	}
}

void report_LED_state(void)
{
  int i = 0;

  Serial.write('L');
  for (i = 0; i < COUNTOF(tree_port); i++) {
	if (i < strlen(THIS_DEVICE_COLOR_MAP)) {
		Serial.write(digitalRead(tree_port[i]) == HIGH ? THIS_DEVICE_COLOR_MAP[i] : '_');
	} else {
		Serial.write(digitalRead(tree_port[i]) == HIGH ? i+'0' : '_');
	}
  }
  Serial.write('$');
  Serial.write('F');
  flasher.report_state();
  Serial.write('S');
  strober.report_state();
  Serial.write('\n');
}

void report_device_state(void)
{
	Serial.write('Q');	// query response
	Serial.write('B');	// device model = busylight
	Serial.write('=');	// settings
	Serial.write('_');	// not implemented for this device
	Serial.write(EEPROM.read(EE_ADDR_USB_SPEED)); // USB speed
	Serial.write('_');	// not implemented for this device
	Serial.write('_');	// not implemented for this device
	Serial.write('$');	// end of settings
	Serial.write(SERIAL_VERSION_STAMP);	// hardware version, firmware version, serial number
	Serial.write('\n'); // end of query response
}
	

	
bool set_baud_rate(byte baud_code) {
	if ((baud_code >= '0' && baud_code <= '9') 
	|| (baud_code >= 'A' && baud_code <= 'C')
	|| (baud_code >= 'a' && baud_code <= 'c')) {
		EEPROM.write(EE_ADDR_USB_SPEED, baud_code);
		start_usb_serial();
		return true;
	}
	return false;
}

// begin: start off the state machine
void CommandStateMachine::begin(void) {
	reset();
}

// reset: reset the state machine back to the start state
void CommandStateMachine::reset(void) {
	state = IdleState;
	command_in_progress = 0;
	buffer_idx = 0;
	LEDset = 0;
	for (int i=0; i<CSM_BUFSIZE; i++) 
		buffer[i] = 0;
}


// accept an input character into the state machine
void CommandStateMachine::accept(int inputchar) {
	int i;

	if (inputchar < 0) {
		return;
	}

	switch (state) {
		case EndState:
		case ErrorState:
			if (inputchar == '\x04') {
				// ^D marks end of command; reset to start next command
				reset();
			}
			// Otherwise stay here until a ^D arrives
			break;

		case IdleState:
			// Start of command
			switch (command_in_progress = inputchar) {
				case '*': 
					state = StrobeState; 
					break;
				case '=': 
					state = SetState; 
					break;
				case '?': 
					report_LED_state();
					end_cmd();
					break;
				case 'f':
				case 'F':
					state = FlashState;
					break;
				case 'q':
				case 'Q':
					report_device_state();
					end_cmd();
					break;
				case 's':
				case 'S':
					state = LightSetState;
					break;
				case 'x':
				case 'X':
					all_off(true);
					end_cmd();
					break;
				default:
					error();
			}
			break;

		case StrobeState:
			if (inputchar == '\x1b' || inputchar == '$') {
				strober.stop();
				for (i=0; i<buffer_idx; i++) {
					strober.append(buffer[i]);
				}
				strober.start();
				end_cmd();
			} else {
				accept_led_name(inputchar);
			}
			break;

		case SetState:
			// ignore input byte (for readerboard compatibility)
			state = SetUSpeedState;
			break;

		case SetUSpeedState:
			if (set_baud_rate(inputchar)) {
				end_cmd();
			} else {
				error();
			}

			break;

		case FlashState:
			if (inputchar == '\x1b' || inputchar == '$') {
				flasher.stop();
				for (i=0; i<buffer_idx; i++) {
					flasher.append(buffer[i]);
				}
				flasher.start();
				end_cmd();
			} else {
				accept_led_name(inputchar);
			}
			break;

		case LightSetState:
			all_off(false);
			if (accept_led_name(inputchar)) {
				set_steady_led(buffer[0]);
			}
			end_cmd();
			break;

		default:
			error();
	}
}

// set_steady_led(code): light up the LED at the indicated position
void set_steady_led(byte code) {
	if (code != SEQUENCE_ERROR && code != SEQUENCE_OFF) {
		if (code < COUNTOF(tree_port)) {
			digitalWrite(tree_port[code], HIGH);
		}
	}
}

// light_code: take a color or position code, return the integer position value or SEQUENCE_OFF or SEQUENCE_ERROR.
byte light_code(byte code) {
	if (code == '_') {
		return SEQUENCE_OFF;
	}

	for (int i=0; i<strlen(THIS_DEVICE_COLOR_MAP); i++) {
		if (code == THIS_DEVICE_COLOR_MAP[i]) {
			return i;
		}
	}

	if (code >= '0' && code <= '9') {
		if ((code - '0') >= COUNTOF(tree_port)) {
			return SEQUENCE_OFF;
		}
		return code - '0';
	}

	return SEQUENCE_ERROR;
}

// accept_led_name: accept an LED position number or symbolic color name
// add the LED position to the CSM's buffer, return if successful
// and if unsuccessful shift to Error state.
bool CommandStateMachine::accept_led_name(int inputchar) {
	byte code;

	if ((code = light_code(inputchar)) == SEQUENCE_ERROR) {
		error();
		return false;
	}
	
	return append_buffer(code);
}

bool CommandStateMachine::append_buffer(byte value) {
	if (buffer_idx >= CSM_BUFSIZE)
		return false;
	buffer[buffer_idx++] = value;
	return true;
}

void loop() {
	//
	// loop forever, adjusting the lights as each
	// command comes in. We will silently ignore
	// invalid commands (with the side effect that
	// it's perfectly fine to throw newlines in the
	// stream).
	//
	flasher.update();
	strober.update();

	if (Serial.available() > 0) {
		csm.accept(Serial.read());
	}
}

// flash a single light on/off, or sequence through a list at the on cadence.
void flash_lights(void)
{
	flasher.advance();
}

void strobe_lights(void)
{
	strober.advance();
}
