/*
** Steve Willoughby <steve@madscience.zone>
** Licensing: BSD 3-clause open-source license
**
** Simple busylight indicator, controlled via USB serial commands.
** To use, open the USB device as a serial port and send single 
** character commands to it:
**
**  Fn...$	Flash one or more lights
**  Sn		Turn on light #n steady
**  *n...$	Strobe one or more lights (or none)
**  X       All lights off
**
**  These legacy commands are still accepted as aliases for new commands:
**  B == S0
**  G == S4
**  Y == S3
**  R == S2
**	2 == S1
**  # == F12$
**  % == F01$
**
** This legacy command is no longer supported (and was never a good idea):
**  ! 
**
** The alphabetic commands may be sent in either case.
** Any other bytes are simply ignored.
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
*/

#include <TimerEvent.h>

//
// Digital output pin numbers for the lights
// (a high output turns on the LEDs)
//                       BL R2 R1  Y  G
const int tree_port[] = {10, 7, 6, 8, 9, 14, 16};

//
// These LightBlinkers handle our flashing and strobing.
// In each case, they sequence through one or more lights
// at each flash.
// 
#define MAX_SEQUENCE (64)
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
	if (sequence_length < MAX_SEQUENCE - 1) {
		sequence[sequence_length++] = v;
	}
}

void LightBlinker::report_state(void)
{
	// report state as cur_state (0 or 1) then "X" (off) or "<index>@<sequence>"
	int i = 0;
	Serial.write(cur_state ? '1' : '0');
	if (sequence_length > 0) {
		Serial.write(cur_index + '0');
		Serial.write('@');
		for (i = 0; i < sequence_length; i++) {
			Serial.write(sequence[i] + '0');
		}
	} else {
		Serial.write('X');
	}
}

void LightBlinker::advance(void)
{
	// If we have a sequence of one item, we will just flash that one on and off
	// If we have no off_period, it's the same as the on_period
	if (sequence_length < 2) {
		if (cur_state) {
			digitalWrite(tree_port[sequence[0]], LOW);
			cur_state = false;
			if (off_period > 0)
				timer.setPeriod(off_period);
		} else {
			digitalWrite(tree_port[sequence[0]], HIGH);
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
		digitalWrite(tree_port[sequence[cur_index]], LOW);
		cur_index = (cur_index + 1) % sequence_length;
		digitalWrite(tree_port[sequence[cur_index]], HIGH);
	}
	else {
		if (cur_state) {
			digitalWrite(tree_port[sequence[cur_index]], LOW);
			timer.setPeriod(off_period);
			cur_state = false;
		}
		else {
			cur_index = (cur_index + 1) % sequence_length;
			digitalWrite(tree_port[sequence[cur_index]], HIGH);
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
		digitalWrite(tree_port[sequence[0]], HIGH);
		timer.reset();
		timer.setPeriod(on_period);
		timer.enable();
	}
	else {
		stop();
	}
}

void flash_lights(void);
void strobe_lights(void);
LightBlinker flasher(200, 0, flash_lights);
LightBlinker strober(50, 2000, strobe_lights);

#define COUNTOF(X) (sizeof(X)/sizeof(X[0]))
	

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
	//
	// Our serial port will be 9600 baud
	//
	flasher.stop();
	strober.stop();
	Serial.begin(9600);
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

void report_state(void)
{
  int i = 0;

  Serial.write('L');
  for (i = 0; i < COUNTOF(tree_port); i++) {
    Serial.write(digitalRead(tree_port[i]) == HIGH ? '1' : '0');
  }
  Serial.write('F');
  flasher.report_state();
  Serial.write('S');
  strober.report_state();
  Serial.write('\n');
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

	const int IDLE=0;		// not waiting for anything
	const int SINGLETON=1;	// waiting for a single light ID
	const int LIST=2;		// waiting for list of IDs

	static byte state = IDLE;
	static byte cmd = 0;	// command being parsed or 0

	if (Serial.available() > 0) {
		int inputvalue = Serial.read();
			
		switch (inputvalue) {
		case 'S':
		case 's':
			state = SINGLETON;
			cmd = (byte)inputvalue;
			break;

		case 'X':
		case 'x':
			state = IDLE;
			all_off(true);
			break;

		case '*':
			state = LIST;
			strober.stop();
			cmd = (byte)inputvalue;
			break;

		case 'F':
		case 'f':
			state = LIST;
			flasher.stop();
			cmd = (byte)inputvalue;
			break;

		case 'B':
		case 'b':
			state = IDLE;
			all_off(true);
			digitalWrite(tree_port[0], HIGH);
			break;

		case 'G':
		case 'g':
			state = IDLE;
			all_off(true);
			digitalWrite(tree_port[4], HIGH);
			break;

		case 'Y':
		case 'y':
			state = IDLE;
			all_off(true);
			digitalWrite(tree_port[3], HIGH);
			break;

		case 'R':
		case 'r':
		case '!':
			state = IDLE;
			all_off(true);
			digitalWrite(tree_port[2], HIGH);
			break;

		case '2':
			if (state == IDLE) {
				state = IDLE;
				all_off(true);
				digitalWrite(tree_port[1], HIGH);
				break;
			}
			// FALLTHRU

		case '0':
		case '1':
		case '3':
		case '4':
		case '5':
		case '6':
			switch (state) {
			case LIST:
				if (cmd == 'f' || cmd == 'F') {
					if (inputvalue >= '0' && inputvalue <= '6') 
						flasher.append(inputvalue - '0');
				}
				else if (cmd == '*') {
					if (inputvalue >= '0' && inputvalue <= '6') 
						strober.append(inputvalue - '0');
				}
				else
					state = IDLE;
				break;

			case SINGLETON:
				if (cmd == 's' || cmd == 'S') {
					all_off(false);
					flasher.stop();
					if (inputvalue >= '0' && inputvalue <= '6') {
						digitalWrite(tree_port[inputvalue - '0'], HIGH);
					}
				}
				state = IDLE;
				break;
			
			default:
				state = IDLE;
			}
			break;

		case '\x1b':
		case '$':
			if (state == LIST) {
				all_off(false);
				if (cmd == 'f' || cmd == 'F') 
					flasher.start();
				else if (cmd == '*') 
					strober.start();
			}
			state = IDLE;
			break;

		case '#':
			state = IDLE;
			all_off(true);
			flasher.stop();
			flasher.append(1);
			flasher.append(2);
			flasher.start();
			break;

		case '%':
			all_off(true);
			state = IDLE;
			flasher.stop();
			flasher.append(0);
			flasher.append(1);
			flasher.start();
			break;

		case '@':
			state = IDLE;
			strober.stop();
			strober.append(4);
			strober.start();
			break;

    case '?':
      state = IDLE;
      report_state();
      break;

		default:
			state = IDLE;
		}
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
