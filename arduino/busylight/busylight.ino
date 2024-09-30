//  ____  _   _ ______   ___     ___ ____ _   _ _____   v2.0.0 firmware
// | __ )| | | / ___\ \ / / |   |_ _/ ___| | | |_   _| 
// |  _ \| | | \___ \\ V /| |    | | |  _| |_| | | |  
// | |_) | |_| |___) || | | |___ | | |_| |  _  | | |  
// |____/ \___/|____/ |_| |_____|___\____|_| |_| |_|  
//
// THIS FIRMWARE IS FOR A PRO MICRO CONTROLLER ATTACHED
// TO A BUSYLIGHT VERSION 2 SHIELD. If you are using a version 1
// shield, it should still work, but since that board does not
// have an RS-485 port, those functions will simply not work.
//
// The Pro Micro (often called the SparkFun Pro Micro, and made by a variety
// of different manufacturers) is based on the ATmeta32U4 
// (we use the 5V, 16MHz version).
//
// In the Arduino IDE, set the board type to "SparkFun Pro Micro",
// The CPU type to "ATmega32U4 (5V, 16 MHz)".
// You may need to add support for this to your board manager by adding
// https://raw.githubusercontent.com/sparkfun/Arduino_Boards/main/IDE_Board_Manager/package_sparkfun_index.json
// to your IDE's preferences.
//
// ===> CHANGE THE FOLLOWING PER DEVICE before flashing. <===
//
// THIS_DEVICE_COLOR_MAP can be empty or contain letter
// codes to represent the colors implemented in light
// positions 0-6. Any left off at the end of the list
// must be addressed only by position numbers in the
// protocol commands.
#define THIS_DEVICE_COLOR_MAP "BRrYG"
//                             ^^^^^^^
//                             0123456
//
// SERIAL_VERSION_STAMP holds the version number of your
// hardware, the version of this firmware code, and the
// unique serial number for this unit. Serial numbers
// B000-B299 are reserved for the author; please use other
// values for your own devices. Serial numbers must be alphanumeric only.
#define SERIAL_VERSION_STAMP "V2.0.0$R2.0.0$SBXXX$"
//                             \___/  \___/  \__/
//                               |      |      |
//                  Hardware version    |      |
//                         Firmware version    |
//                                 Serial number
//
// These are variable-length values, each terminated by a dollar-sign ($).

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
**                  BLUE   B  ==================	#0
**                                    ||
**                  RED 2  R  ==================	#1
**                                    ||
**                  RED 1  r  ==================	#2
**                                    ||
**                  YELLOW Y  ==================	#3
**                                    ||
**                  GREEN  G  ==================	#4
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
** Commands may be received over the USB connection from a host computer,
** or over an RS-485 serial network.
**
** USB
** All commands received on the USB port are terminated with a ^D ($04) 
** byte. If an error occurs, all data are ignored until a ^D is received.
** Extra data after a command (but before the terminating ^D) is ignored.
**
** RS-485
** Commands received via RS-485 start with a binary
** header to be compatible with Lumos devices on the same network.
**
** 1000aaaa                                              turn all LEDs off. [1]
** 1001aaaa <command>                                    do <command> as described below. [1][3]
** 1011aaaa 0000nnnn 0000xxxx ... 0000xxxx <command>     do <command> as described below. [2][3]
**                   \________<n>________/
**
** [1] if aaaa is the device address or the global address, the unit will
**     respond to the command.
**
** [2] aaaa must be the global address; if the unit address is in the
**     list of addresses following the start byte, it will respond.
**
** [3] All following bytes must have MSB=0 and use the escape sequences:
**       01111110 0xxxxxxx     -> 1xxxxxxx (set MSB in next byte)
**       01111111 0xxxxxxx     -> 0xxxxxxx (take next byte literally)
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
** State Machine
**     -> ERR       signal error condition and go to ERROR state
**     -> END       go to END state
**     +            change transition and then re-examine current input byte
**                          
**                    
**                                                                             ____ 
**          $8a/$8g (485) _      $Bg (485)        _________  N    ____________|_   | Ad[0]-Ad[N-2]
**                       | |  +----------------->|Collect N|---->|CollectAddress|<-+
**        MSB=1 (485)+   | |  |                  |_________|     |______________|
**  _____  ^D (USB)     _|_V__|_   $9a/$Dg (485)  _____             |Ad[N-1]
** |ERROR|----------->||Idle    ||-------------->|Start|<-----------+
** |_____|<-----------||________||               |_____|
**   ^ |       *            |                        |
**   |_|*                   |<-----------------------+     _ 
**          ?/Q             |            ____             | |*
**  END <-------------------+ '*'   ____|_   | led       _V_|_  ^D
**                          +----->|Strobe|<-+      $   |END  |-----------> Idle
**   _____    =             |      |______|------------>|_____|
**  |Set  |<----------------+ 
**  |_____|                 |
**    | Ad/_                |
**   _V_____                |
**  |SetUspd|               |
**  |_______|               |
**    | speed               |
**   _V_____                |           ____
**  |SetRspd|               |  F    ___|_   |led
**  |_______|               +----->|Flash|<-+      $
**    | speed               |      |_____|----------> END 
**   _V_____                |  S    ________ led
**  |SetAg  |               +----->|LightSet|----------> END
**  |_______|               |      |________| 
**    | addr                |  X
**    V                     +-----> END
**   END
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
**     Q B = <address> <usb-speed> <rs-485-speed> <global-address> $ V <hwversion> $ R <romversion> $ S <serialno> $ \n
**
*/

#include <TimerEvent.h>
#include <EEPROM.h>

#define COUNTOF(X) (sizeof(X)/sizeof(X[0]))

//
// EEPROM locations
// $00 0x4B
// $01 baud rate code (USB)
// $02 baud rate code (RS-485 if on separate USART) or 0 if port disabled
// $03 unit address or UNIT_DISABLED
// $04 global address or UNIT_DISABLED
//
#define EE_ADDR_SENTINEL  (0x00) /* EEPROM $00: constant sentinel byte */
#define EE_ADDR_USB_SPEED (0x01) /* EEPROM $01: usb baud rate code */
#define EE_ADDR_485_SPEED (0x02) /* EEPROM $02: rs-485 baud rate code; NOT USED if common serial port */
#define EE_ADDR_485_ADDR  (0x03) /* EEPROM $03: rs-485 unit address */
#define EE_ADDR_GLOB_ADDR (0x04) /* EEPROM $04: rs-485 global address */
#define EE_VALUE_SENTINEL (0xb1) /* EEPROM sentinal byte value */
#define EE_DEFAULT_SPEED  ('5')  /* default baud rate code */
#define UNIT_DISABLED     (0xff) /* address code if unit unit has no address */

//
// Digital output pin numbers for the lights
// (a high output turns on the LEDs)
//                       BL R2 R1  Y  G
const int tree_port[] = {10, 7, 6, 8, 9, 14, 16};
//                       #0 #1 #2 #3 #4  #5  #6

//
// Other output pin numbers
// 
#define PIN_485_DRIVER_ENABLE	 (2)	/* 1=RS-485 driver enabled */
#define PIN_485_RECEIVER_DISABLE (3)	/* 0=RS-485 receiver enabled */
bool rs_485_enabled = false;			/* should we even try to talk to the RS-485 port? */

//
// These LightBlinkers handle our flashing and strobing.
// In each case, they sequence through one or more lights
// at each flash.
// 
#define MAX_SEQUENCE    (64)
#define SEQUENCE_ERROR (254)	/* error in sequence codes */
#define SEQUENCE_OFF   (255)    /* spot in sequence where LEDs are off */
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
		CollectAddressState,
		CollectNState,
		EndState,
		ErrorState,
		FlashState,
		IdleState,
		LightSetState,
		SetAgState,
		SetState,
		SetRSpeedState,
		SetUSpeedState,
		StartState,
		StrobeState,
	} state;
	byte LEDset;
	byte command_in_progress;
	byte buffer[CSM_BUFSIZE];
	byte address_count;
	byte buffer_idx;
	bool source_485;
	bool next_byte_msb;
	bool next_byte_literal;
public:
	void accept(int inputchar, bool from_485=false);
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
byte my_address = UNIT_DISABLED;
byte global_address = UNIT_DISABLED;

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
	my_address = EEPROM.read(EE_ADDR_485_ADDR);
	global_address = EEPROM.read(EE_ADDR_GLOB_ADDR);
	
	// By default, the hardware disables the RS-485 transceiver chip.
	// Here, we'll activate the control lines and either set the transceiver
	// to receive or leave it disabled.
	pinMode(PIN_485_DRIVER_ENABLE, OUTPUT);
	digitalWrite(PIN_485_DRIVER_ENABLE, LOW);
	//            __
	// We set the RE line high before enabling it as an output. This
	// will ensure that it's sending a high signal from the start (and
	// will cause the pin's internal pull-up resistor to be activated
	// as well during the time before it is switched to output mode).
	digitalWrite(PIN_485_RECEIVER_DISABLE, HIGH);
	pinMode(PIN_485_RECEIVER_DISABLE, OUTPUT);
	if (my_address != UNIT_DISABLED) {
		enable_485_serial();
	}
	digitalWrite(tree_port[0], LOW);
	csm.begin();
}

void default_eeprom_settings(void) {
	if (EEPROM.read(EE_ADDR_SENTINEL) != EE_VALUE_SENTINEL) {
		// apparently unset; set to "factory defaults"
		EEPROM.write(EE_ADDR_USB_SPEED, EE_DEFAULT_SPEED);
		EEPROM.write(EE_ADDR_485_SPEED, 0); // 0=disabled
		EEPROM.write(EE_ADDR_485_ADDR, UNIT_DISABLED);
		EEPROM.write(EE_ADDR_GLOB_ADDR, UNIT_DISABLED);
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
	disable_485_serial();
}

long decode_baud_rate(byte code) {
	long speed = 9600;

	switch (code) {
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
	return speed;
}

void start_usb_serial(void) {
	long speed = decode_baud_rate(EEPROM.read(EE_ADDR_USB_SPEED));
	Serial.begin(speed);
	while (!Serial);
}

void enable_485_serial(void) {
	/* turn on the transceiver in receiver mode */
	digitalWrite(PIN_485_DRIVER_ENABLE, LOW);
	digitalWrite(PIN_485_RECEIVER_DISABLE, LOW);

	/* tell the rest of the firmware to pay attention to the port */
	rs_485_enabled = true;

	/* set up the Serial1 interface and set the baud rate on the UART */
	start_485_serial();
}

void disable_485_serial(void) {
	/* turn off the transceiver chip entirely */
	digitalWrite(PIN_485_DRIVER_ENABLE, LOW);
	digitalWrite(PIN_485_RECEIVER_DISABLE, HIGH);

	/* tell the rest of the firmware to ignore the port */
	rs_485_enabled = false;
}

void start_485_serial(void) {
	byte speed_code = EEPROM.read(EE_ADDR_485_SPEED);
	if (speed_code > 0) {
		long speed = decode_baud_rate(speed_code);
		Serial1.begin(speed);
		while (!Serial1);
	} else {
		disable_485_serial();
	}
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
	byte b;
	Serial.write('Q');	// query response
	Serial.write('B');	// device model = busylight
	Serial.write('=');	// settings
	if (my_address == UNIT_DISABLED) {
		Serial.write('_');
	} else if (my_address < 64) {
		Serial.write(my_address + '0');
	} else {
		Serial.write('*');
	}
	Serial.write(EEPROM.read(EE_ADDR_USB_SPEED)); // USB speed
	if ((b = EEPROM.read(EE_ADDR_485_SPEED)) == 0) {
		Serial.write('_');
	} else {
		Serial.write(b);
	}
	if (global_address == UNIT_DISABLED) {
		Serial.write('_');
	} else if (global_address < 16) {
		Serial.write(global_address + '0');
	} else {
		Serial.write('*');
	}
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

bool set_485_baud_rate(byte baud_code) {
	if (baud_code == 0 || my_address == UNIT_DISABLED) {
		EEPROM.write(EE_ADDR_485_SPEED, 0);
		return true;
	}
	if ((baud_code >= '0' && baud_code <= '9') 
	|| (baud_code >= 'A' && baud_code <= 'C')
	|| (baud_code >= 'a' && baud_code <= 'c')) {
		EEPROM.write(EE_ADDR_485_SPEED, baud_code);
		start_485_serial();
		return true;
	}
	return false;
}

// begin: start off the state machine
void CommandStateMachine::begin(void) {
	source_485 = false;
	reset();
}

// reset: reset the state machine back to the start state
void CommandStateMachine::reset(void) {
	state = IdleState;
	address_count = 0;
	command_in_progress = 0;
	next_byte_literal = false;
	next_byte_msb = false;
	buffer_idx = 0;
	LEDset = 0;
	for (int i=0; i<CSM_BUFSIZE; i++) 
		buffer[i] = 0;
}


// accept an input character into the state machine
void CommandStateMachine::accept(int inputchar, bool from_485) {
	int i;

	// if we were in the middle of a command and suddenly flipped
	// input source, signal that as an error
	if (from_485 && !source_485) {			// switching to RS-485
		if (state != IdleState) {
			error();
		}
		source_485 = true;
	} else if (!from_485 && source_485) {	// switching to USB
		if (state != IdleState) {
			error();
		}
		source_485 = false;
	}

	if (inputchar < 0) {
		return;
	}

	// If in End or Error state, wait for command boundary then move to Idle state.
	if (state == EndState || state == ErrorState) {
		if (source_485) {
			if (inputchar & 0x80) {
				reset();
				/* continue to interpret this byte below */
			}
			else {
				return;
			}
		} else {
			if (inputchar == '\x04') {
				reset();
			}
			return;
		}
	}

	if (source_485 && (inputchar & 0x80) && state != IdleState) {
		// encountered sudden start of command
		error();
		reset();
		// continue to interpret this byte below 
	}

	// Handle start of RS-485 command block
	if (source_485) {
		byte target;
		if (state == IdleState) {
			if (inputchar & 0x80) {
				/* start of new command */
				if ((target = (inputchar & 0x0f)) == my_address || target == global_address) {
					if ((target & 0x70) == 0x00) {	// 1000aaaa: turn off all lights
						all_off(true);
						end_cmd();
						return;
					}
					if ((target & 0x70) == 0x10) {	// 1001aaaa: normal command addressed to us (or global)
						state = StartState;
						return;
					}
					if ((target & 0x70) == 0x30) {	// 1011aaaa: start of multi-address block
						state = CollectNState;
						return;
					}
					error();
					return;
				} else {
					/* not addressed to us */
					end_cmd();
					return;
				}
			} else {
				/* data byte while waiting for start of command */
				end_cmd();
				return;
			}
		} else {
			/* RS-485 byte received while working on command; apply escape codes */
			if (next_byte_literal) {
				next_byte_literal = false;
			} else {
				if (next_byte_msb) {
					next_byte_msb = false;
					inputchar |= 0x80;
				} else {
					if (inputchar == 0x7e) {
						next_byte_msb = true;
						return;
					}
					if (inputchar == 0x7f) {
						next_byte_literal = true;
						return;
					}
				}
			}
		}
	}

	switch (state) {
		case IdleState:
		case StartState:
			// Start of command
			switch (command_in_progress = inputchar) {
				case '*': 
					state = StrobeState; 
					break;
				case '=': 
					if (source_485) {
						error();
						break;
					}
					state = SetState; 
					break;
				case '?': 
					if (!source_485) {
						report_LED_state();
					}
					end_cmd();
					break;
				case 'f':
				case 'F':
					state = FlashState;
					break;
				case 'q':
				case 'Q':
					if (!source_485) {
						report_device_state();
					}
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
			if (inputchar == '_') {
				my_address = UNIT_DISABLED;
			} else if ((my_address = (inputchar - '0')) < 0 || my_address > 63) {
				error();
				break;
			}
			EEPROM.write(EE_ADDR_485_ADDR, my_address);
			state = SetUSpeedState;
			break;

		case SetAgState:
			if (inputchar == '_') {
				global_address = UNIT_DISABLED;
			} else if ((global_address = (inputchar - '0')) < 0 || global_address > 15) {
				error();
				break;
			}
			EEPROM.write(EE_ADDR_GLOB_ADDR, global_address);

			if (!set_baud_rate(buffer[0])) {
				error();
				break;
			}
			if (!set_485_baud_rate(buffer[1])) {
				error();
				break;
			}
			if (my_address == UNIT_DISABLED) {
				disable_485_serial();
			} else {
				enable_485_serial();
			}
			end_cmd();
			break;

		case SetUSpeedState:
			buffer[0] = inputchar;
			state = SetRSpeedState;
			break;

		case SetRSpeedState:
			buffer[1] = inputchar;
			state = SetAgState;
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

		case CollectNState:
			if (inputchar == 0) {
				state = StartState;
			} else if (inputchar > 15) {
				error();
			} else {
				state = CollectAddressState;
				address_count = inputchar;
			}
			break;

		case CollectAddressState:
			if (!append_buffer(inputchar)) {
				error();
				break;
			}
			if (--address_count == 0) {
				for (i=0; i<buffer_idx; i++) {
					if ((buffer[i] & 0x3f) == my_address || (buffer[i] & 0x3f) == global_address) {
						buffer_idx = 0;
						state = StartState;
						return;
					}
				}
				end_cmd();
				return;
			}
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
		csm.accept(Serial.read(), false);
	}
	if (rs_485_enabled && Serial1.available() > 0) {
		csm.accept(Serial1.read(), true);
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
