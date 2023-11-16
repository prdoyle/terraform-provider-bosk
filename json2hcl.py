#!/usr/bin/env python3

# Really just changes ":" to "=" for map entries.
# There seems to be no convenient way to eliminate the quotes around the keys.

import json
from sys import stdin, stdout

content = json.load( stdin )
stdout.write("jsonencode(")
json.dump( content, stdout, indent='\t', separators=(',', ' = '))
stdout.write(")")

