import os, sys, time, subprocess
from Xlib import X, display
from Xlib.ext import xtest

out = sys.argv[1]
d = display.Display()
root = d.screen().root

def move(x, y):
    xtest.fake_input(d, X.MotionNotify, x=x, y=y); d.sync(); time.sleep(0.15)

def click(x, y, button=1):
    move(x, y)
    xtest.fake_input(d, X.ButtonPress, button); d.sync(); time.sleep(0.08)
    xtest.fake_input(d, X.ButtonRelease, button); d.sync(); time.sleep(2.5)

def key(name, hold=0.1):
    kc = d.keysym_to_keycode({'w': 0x77, 'e': 0x65, 'Escape': 0xff1b, 'space': 0x20, 's': 0x73}[name])
    xtest.fake_input(d, X.KeyPress, kc); d.sync(); time.sleep(hold)
    xtest.fake_input(d, X.KeyRelease, kc); d.sync(); time.sleep(2.5)

def shot(name):
    subprocess.run(['import', '-window', 'root', os.path.join(out, name + '.png')], check=True)
    print('shot', name, flush=True)

move(640, 300); time.sleep(1.5)
shot('d1_title')
click(640, 354)                  # Singleplayer
shot('d2_worlds')
click(640, 255)                  # the second world: New World
click(410, 594)                  # Play Selected World (left of Create New World)
time.sleep(8)
shot('d3_ingame')
key('w', 4)                    # walk forward
shot('d4_walked')
key('e')                         # creative inventory
shot('d5_inventory')
click(398, 234)                  # first item into the selected slot
key('e')
key('Escape')                    # pause
shot('d6_pause')
click(640, 281)                  # Back to Game
time.sleep(1)
shot('d7_back')
click(640, 360, 3)               # place a block where the cross-hair points
key('Escape')                    # pause again
click(640, 498)                  # Save and Quit to Title
shot('d8_title_after_save')
click(487, 570)                  # Options...
shot('d9_options')
click(560, 114)                  # FOV slider, far to the right
click(640, 665)                  # Done: saves the options
shot('d10_title_again')
