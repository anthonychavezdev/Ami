"""One-to-one call between two emulators: luis on A calls ana on B from her
profile; ringing, accepting, screen share from A, hanging up.

    python3 mobile/e2e/direct_call.py [emulator-5554 [emulator-5556]]
"""
import sys, time

from ami import Dev, report, step

A = Dev(sys.argv[1] if len(sys.argv) > 1 else 'emulator-5554', 'A')
B = Dev(sys.argv[2] if len(sys.argv) > 2 else 'emulator-5556', 'B')

B.restart()
A.restart()
step('B logs in as ana', lambda: B.login_as('ana'))
step('A is luis (logs in if needed)', lambda: A.login_as('luis') if A.find('Username', exact=True, timeout=5) else bool(A.find("What's on your mind?", timeout=30)))


def a_call():
    A.tap_text('Friends', exact=True)
    A.tap_text('Ana García')
    A.tap_text('Call', exact=True)
    time.sleep(1)
    A.allow()
    return bool(A.find('Calling…', exact=True, timeout=20))


step("A calls Ana from her profile", a_call)
step('B is rung by Luis', lambda: bool(B.find('Incoming voice call', exact=True, timeout=20) and B.find('Luis Martínez', timeout=5)))
B.shot('direct-B-ringing')


def b_accept():
    B.tap_text('Accept', exact=True)
    time.sleep(1)
    B.allow()
    return bool(B.find('Voice call', exact=True, timeout=30))


step('B accepts; the call is connected', b_accept)
step('A is connected too', lambda: bool(A.find('Voice call', exact=True, timeout=20)))
A.shot('direct-A-active')
B.shot('direct-B-active')


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
B.shot('direct-B-stage')
step('B renders the shared screen (coloured pixels in the stage area)', lambda: sum(1 for v in B.bright_pixels([(x, y) for x in range(380, 700, 10) for y in range(420, 1050, 12)]) if v > 120) > 10)
step('A stops sharing; B is back to a plain voice call', lambda: (A.tap_text('Stop share', exact=True), B.find('Voice call', exact=True, timeout=20))[1])
step('A hangs up; B is back on the Feed it was rung on', lambda: (A.tap_text('End', exact=True), B.find("What's on your mind?", timeout=20))[1])
step("A is back on Ana's profile", lambda: bool(A.find('Unfriend', exact=True, timeout=15)))
report()
