"""Group call between two emulators (luis on A, ana on B) plus a signaling-only
third participant (diego) over the WebSocket: ringing, joining, screen share
from A, camera on B, leaving.

    python3 mobile/e2e/group_call.py [emulator-5554 [emulator-5556]]
"""
import sys, time

from ami import WS, Dev, check, login, report, step

A = Dev(sys.argv[1] if len(sys.argv) > 1 else 'emulator-5554', 'A')
B = Dev(sys.argv[2] if len(sys.argv) > 2 else 'emulator-5556', 'B')
ROOM = 'g:1'  # Fútbol los domingos (server/demo.go)

B.restart()
A.restart()
step('B logs in as ana', lambda: B.login_as('ana'))
step('A is luis (logs in if needed)', lambda: A.login_as('luis') if A.find('Username', exact=True, timeout=5) else bool(A.find("What's on your mind?", timeout=30)))


def a_start():
    A.tap_text('Messages', exact=True)
    A.tap_text('Fútbol los domingos')
    A.tap_text('Group voice call', exact=True)
    time.sleep(1)
    A.allow()
    return bool(A.find('Waiting for others to join', timeout=20))


step('A starts a group voice call and waits alone', a_start)
A.shot('call-A-waiting')
step('B is rung with the caller and the group', lambda: bool(B.find('Luis Martínez is calling', timeout=20) and B.find('Fútbol los domingos', timeout=5)))
B.shot('call-B-ringing')


def b_accept():
    B.tap_text('Accept', exact=True)
    time.sleep(1)
    B.allow()
    return bool(B.find('2 in the call', timeout=30))


step('B accepts; both connected (2 in the call)', b_accept)
step('A sees 2 in the call', lambda: bool(A.find('2 in the call', timeout=20)))
A.shot('call-A-two')
B.shot('call-B-two')
w = WS(login('diego'))
w.expect('hello')
w.send({'type': 'call.join', 'room': ROOM})
j = w.expect('call.joined')
check('diego joins over the WebSocket and sees both participants', sorted(p['username'] for p in j['participants']) == ['ana', 'luis'])
step('A counts 3 in the call', lambda: bool(A.find('3 in the call', timeout=20)))
step('B counts 3 in the call', lambda: bool(B.find('3 in the call', timeout=20)))
A.shot('call-A-three')
B.shot('call-B-three')
w.send({'type': 'call.leave', 'room': ROOM})
w.close()
step('A back to 2 in the call after diego leaves', lambda: bool(A.find('2 in the call', timeout=20)))


def a_share():
    A.tap_text('Share screen', exact=True)
    time.sleep(1.5)
    for _ in range(6):
        if A.find('You are sharing your screen', timeout=2):
            return True
        A.allow()
        time.sleep(1)
    return bool(A.find('You are sharing your screen', timeout=25))


step('A shares the screen', a_share)
step('B sees that Luis is sharing (stage layout)', lambda: bool(B.find('Luis Martínez is sharing their screen', timeout=25)))
time.sleep(3)
B.shot('call-B-stage')
A.shot('call-A-sharing')
step('B renders the shared screen (coloured pixels in the stage area)', lambda: sum(1 for v in B.bright_pixels([(x, y) for x in range(380, 700, 10) for y in range(420, 1050, 12)]) if v > 120) > 10)
step('A stops sharing; B is back to 2 in the call', lambda: (A.tap_text('Stop share', exact=True), B.find('2 in the call', timeout=20))[1])


def b_camera():
    B.tap_text('Camera', exact=True)
    time.sleep(1)
    B.allow()
    return bool(B.find('Camera off', exact=True, timeout=15))


step('B turns the camera on', b_camera)
time.sleep(5)
A.shot('call-A-video')
step("A shows B's video (bright pixels full screen)", lambda: max(A.bright_pixels([(540, 1000), (540, 1400), (300, 1200), (780, 1200)])) > 60)
step('A leaves; the chat is back', lambda: (A.tap_text('Leave', exact=True), A.find('Message', exact=True, timeout=15))[1])
step('B is alone again', lambda: bool(B.find('Waiting for others to join', timeout=20)))
step('B leaves (back to the Feed it was rung on)', lambda: (B.tap_text('Leave', exact=True), B.find("What's on your mind?", timeout=15))[1])
report()
