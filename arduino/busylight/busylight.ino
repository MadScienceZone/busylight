/*
** Steve Willoughby <steve@madscience.zone>
** vi:set ai sm nu ts=4 sw=4:
** Licensing: BSD 3-clause open-source license
**
** Simple busylight indicator, controlled via USB serial commands.
** To use, open the USB device as a serial port and send single 
** character commands to it:
**
**  X all lights off
**  B only blue layer on
**  G only green layer on
**  Y only yellow layer on
**  R only red layer #1 on
**  2 only red layer #2 on
**  ! only both red layers on
**  # alternately flash red layers
**  % alternately flash blue and red #2
**
** The alphabetic commands may be sent in either case.
** Any other bytes are simply ignored.
**
** The physical LED tree is stacked like so:
**
**                    BLUE    ==================
**                                    ||
**                    RED 2   ==================
**                                    ||
**                    RED 1   ==================
**                                    ||
**                    YELLOW  ==================
**                                    ||
**                    GREEN   ==================
**                                    ||
**                                    ||
**                                    ||
**                                    ||
**                                    ||
**                                    ||
**                                    ||
*/

//
// Digital output pin numbers for the lights
// (a high output turns on the LEDs)
//
const int tree_green  = 9;
const int tree_yellow = 8;
const int tree_red_1  = 6;
const int tree_red_2  = 7;
const int tree_blue   = 10;
//
// tree_flash controls our two flashing modes.
// if 0, no flashing is done and whatever light
// is currently on (if any) stays steadily on.
//
// Otherwise, if set to 1 or 2, the two red lights
// will alternately flash (tree_flash will bounce
// between values 1 and 2 depending on which is
// currently lit; setting its value back to 0 stops
// the flashing).
//
// Likewise with values 3 and 4, which perform an
// alternating flash between the top red and the blue
// lights.
//
static int tree_flash = 0;

void setup() {
	//
	// digital output mode setting
	//
	pinMode(tree_green, OUTPUT);
	pinMode(tree_yellow, OUTPUT);
	pinMode(tree_red_1, OUTPUT);
	pinMode(tree_red_2, OUTPUT);
	pinMode(tree_blue, OUTPUT);
	//
	// Cycle through all the lights at power-on
	// to test that they all work
	//
	all_off(true);
	digitalWrite(tree_blue, HIGH);
	delay(200);
	all_off(true);
	digitalWrite(tree_red_2, HIGH);
	delay(200);
	all_off(true);
	digitalWrite(tree_red_1, HIGH);
	delay(200);
	all_off(true);
	digitalWrite(tree_yellow, HIGH);
	delay(200);
	all_off(true);
	digitalWrite(tree_green, HIGH);
	delay(200);
	all_off(true);
	//
	// Our serial port will be 9600 baud
	//
	Serial.begin(9600);
}

//
// all_off(): reset device to all off, no flashing
//
void all_off(bool reset_state) {
	if (reset_state) {
		tree_flash = 0;
	}
	digitalWrite(tree_green, LOW);
	digitalWrite(tree_yellow, LOW);
	digitalWrite(tree_red_1, LOW);
	digitalWrite(tree_red_2, LOW);
	digitalWrite(tree_blue, LOW);
}

void loop() {
	//
	// loop forever, adjusting the lights as each
	// command comes in. We will silently ignore
	// invalid commands (with the side effect that
	// it's perfectly fine to throw newlines in the
	// stream).
	//
	while (Serial.available() > 0) {
		switch (Serial.read()) {
		case 'B':
		case 'b':
			all_off(true);
			digitalWrite(tree_blue, HIGH);
			break;
		case 'G':
		case 'g':
			all_off(true);
			digitalWrite(tree_green, HIGH);
			break;
		case 'Y':
		case 'y':
			all_off(true);
			digitalWrite(tree_yellow, HIGH);
			break;
		case 'R':
		case 'r':
			all_off(true);
			digitalWrite(tree_red_1, HIGH);
			break;
		case '2':
			all_off(true);
			digitalWrite(tree_red_2, HIGH);
			break;
		case '!':
			// WARNING: This is the only mode which turns on
			// ======== two lights at once. That might push
			//          the current drain too close to the
			//          maximum output of some USB ports.
			all_off(true);
			digitalWrite(tree_red_1, HIGH);
			digitalWrite(tree_red_2, HIGH);
			break;
		case 'X':
		case 'x':
			all_off(true);
			break;
		case '#':
			tree_flash = 1;
			break;
		case '%':
			tree_flash = 3;
			break;
		case '@':
			tree_flash |= 0x40;
			break;
		}
	}
	//
	// If we're in flashing mode, move to the next
	// state and wait until time for the next state
	// change. This should be fine as long as the 
	// delays are very small, since we will wait
	// that long before reading the next input
	// from the serial port.
	//
	// tree_flash
	//  -----001 -> -----010	red2->red1
	//  -----010 -> -----001    red1->red2
	//  -----011 -> -----100	red2->blue
	//  -----100 -> -----011	blue->red2
	//  -1------ -> -1------    flash green
	//
#define TREE_FLASH_GREEN	0x40
#define TREE_FLASH_MODE		0x07

	if ((tree_flash & TREE_FLASH_MODE) == 1) {
		all_off(false);
		tree_flash = (tree_flash & ~TREE_FLASH_MODE) | 2;
		digitalWrite(tree_red_1, HIGH);
		delay(200);
		if (tree_flash & TREE_FLASH_GREEN) {
			digitalWrite(tree_green, HIGH);
			delay(50);
			digitalWrite(tree_green, LOW);
		}
	} else if ((tree_flash & TREE_FLASH_MODE) == 2) {		
		all_off(false);
		tree_flash = (tree_flash & ~TREE_FLASH_MODE) | 1;
		digitalWrite(tree_red_2, HIGH);
		delay(200);
	} else if ((tree_flash & TREE_FLASH_MODE) == 3) {
		all_off(false);
		tree_flash = (tree_flash & ~TREE_FLASH_MODE) | 4;
		digitalWrite(tree_blue, HIGH);
		delay(200);
	} else if ((tree_flash & TREE_FLASH_MODE) == 4) {
		all_off(false);
		tree_flash = (tree_flash & ~TREE_FLASH_MODE) | 3;
		digitalWrite(tree_red_2, HIGH);
		delay(200);
	} else if (tree_flash & TREE_FLASH_GREEN) {
		digitalWrite(tree_green, HIGH);
		delay(50);
		digitalWrite(tree_green, LOW);
		delay(2000);
	}
}
