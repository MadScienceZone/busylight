const int tree_green  = 6
const int tree_yellow = 7
const int tree_red_1  = 8
const int tree_red_2  = 9

void setup() {
	Serial.begin(9600);
	pinMode(tree_green, OUTPUT);
	pinMode(tree_yellow, OUTPUT);
	pinMode(tree_red_1, OUTPUT);
	pinMode(tree_red_2, OUTPUT);
	all_off();
}

void all_off() {
	digitalWrite(tree_green, LOW);
	digitalWrite(tree_yellow, LOW);
	digitalWrite(tree_red_1, LOW);
	digitalWrite(tree_red_2, LOW);
}

void loop() {
	while (Serial.available() > 0) {
		switch (Serial.read()) {
			case 'G':
			case 'g':
				all_off();
				digitalWrite(tree_green, HIGH);
				break;
			case 'Y':
			case 'y':
				all_off();
				digitalWrite(tree_yellow, HIGH);
				break;
			case 'R':
			case 'r':
				all_off();
				digitalWrite(tree_red_1, HIGH);
				break;
			case '!':
				all_off();
				digitalWrite(tree_red_1, HIGH);
				digitalWrite(tree_red_2, HIGH);
				break;
			case 'X':
			case 'x':
				all_off();
		}
	}
}
