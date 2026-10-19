"""Group chat on one emulator: luis opens the football group's chat, sends a
message, and sees one ana sends from elsewhere arrive live.

    python3 mobile/e2e/group_chat.py [emulator-5554]
"""
import os, sys

from ami import Dev, api, check, group_id, login, report, step

A = Dev(sys.argv[1] if len(sys.argv) > 1 else 'emulator-5554', 'A')
A.restart()
step('logged in as luis', lambda: A.login_as('luis'))
step('Messages tab lists the football group chat', lambda: (A.tap_text('Messages', exact=True), A.find('Fútbol los domingos', timeout=20))[1])
A.shot('gc-messages-tab')
step('group chat opens with sender names', lambda: (A.tap_text('Fútbol los domingos'), A.find('Llego 10 minutos tarde', timeout=20) and A.find('Mateo Rodríguez', timeout=5) and A.find('Group details', timeout=5))[1])


def send():
    A.tap(A.find('Message', exact=True, timeout=10))
    A.type_text('Hola desde Android')
    A.hide_kb()
    A.tap_text('Send', exact=True)
    return A.find('Hola desde Android', exact=True, timeout=15)


step('send a group message from Android', send)
text = 'Hola desde otro dispositivo ' + os.urandom(2).hex()
ana = login('ana')
api('POST', f"/api/groups/{group_id(ana, 'Fútbol los domingos')}/messages", {'body': text}, token=ana)
step('a message from another device arrives live with its sender', lambda: A.find(text, timeout=30) and A.find('Ana García', timeout=5))
A.shot('gc-chat-after')
report()
