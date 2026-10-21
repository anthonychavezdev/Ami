"""Shared helpers for the end-to-end scripts: drive the Android app on an
emulator through adb + uiautomator, talk to the server's API and WebSocket,
and keep a pass/fail tally. See README.md next to this file."""
import base64, html, json, os, re, shutil, socket, struct, subprocess, sys, tempfile, time, urllib.request

ADB = shutil.which('adb') or os.path.join(os.environ.get('ANDROID_HOME', ''), 'platform-tools', 'adb')
BASE = os.environ.get('BASE', 'http://127.0.0.1:8080')  # the dev server
OUT = os.environ.get('E2E_OUT', tempfile.gettempdir())  # where screenshots go
PACKAGE = 'xyz.anthonychavez.ami'
PASSWORD = 'amigo-demo'  # every demo account (server/demo.go)

R = []


def check(name, cond):
    R.append(bool(cond))
    print(('PASS ' if cond else 'FAIL ') + name, flush=True)


def step(name, fn):
    try:
        check(name, fn())
    except Exception as e:  # a missing element reads as a failed step, not a crash
        check(f'{name} ({e})', False)


def report():
    print(f'\n{sum(R)}/{len(R)} passed')
    sys.exit(0 if all(R) else 1)


def api(method, path, body=None, token=None):
    r = urllib.request.Request(BASE + path, data=json.dumps(body).encode() if body is not None else None, method=method)
    r.add_header('Content-Type', 'application/json')
    if token:
        r.add_header('Authorization', 'Bearer ' + token)
    with urllib.request.urlopen(r, timeout=10) as resp:
        return json.load(resp)


def login(user):
    return api('POST', '/api/login', {'username': user, 'password': PASSWORD})['token']


def group_id(token, name):
    """The id of one of the token's groups, by name (ids are UUIDs, so they are looked up, never typed)."""
    return next(g['id'] for g in api('GET', '/api/groups?mine=1', token=token) if g['name'] == name)


class Dev:
    """One emulator (or phone) by adb serial; `tag` names it in messages."""

    def __init__(self, serial, tag):
        self.s, self.tag = serial, tag

    def sh(self, *a):
        return subprocess.run([ADB, '-s', self.s, *a], capture_output=True, text=True).stdout

    def dump(self):
        """Every node on screen as (text, content-desc, centre x, centre y)."""
        self.sh('shell', 'uiautomator', 'dump', '/sdcard/ui.xml')
        x = self.sh('shell', 'cat', '/sdcard/ui.xml')
        out = []
        for m in re.finditer(r'<node [^>]*>', x):
            n = m.group(0)
            t = html.unescape((re.search(r' text="([^"]*)"', n) or [0, ''])[1])
            d = html.unescape((re.search(r'content-desc="([^"]*)"', n) or [0, ''])[1])
            b = re.search(r'bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"', n)
            if b:
                x1, y1, x2, y2 = map(int, b.groups())
                out.append((t, d, (x1 + x2) // 2, (y1 + y2) // 2))
        return out

    def find(self, sub, exact=False, timeout=15):
        """Centre of the first element whose text or label matches, polling until timeout."""
        end = time.time() + timeout
        while time.time() < end:
            hits = [(x, y) for t, d, x, y in self.dump() if (sub == t or sub == d) if exact] if exact else [(x, y) for t, d, x, y in self.dump() if sub in t or sub in d]
            if hits:
                return hits[0]
            time.sleep(1)
        return None

    def tap(self, p):
        self.sh('shell', 'input', 'tap', str(p[0]), str(p[1]))
        time.sleep(1.2)

    def tap_text(self, sub, **k):
        p = self.find(sub, **k)
        assert p, f'{self.tag}: not found: {sub}'
        self.tap(p)
        return p

    def type_text(self, s):
        self.sh('shell', 'input', 'text', s.replace(' ', '%s'))
        time.sleep(0.5)

    def hide_kb(self):
        self.sh('shell', 'input', 'keyevent', '111')
        time.sleep(0.5)

    def shot(self, name):
        with open(os.path.join(OUT, f'{name}.png'), 'wb') as f:
            subprocess.run([ADB, '-s', self.s, 'exec-out', 'screencap', '-p'], stdout=f)

    def bright_pixels(self, points):
        """Max channel value at each (x, y); tells a rendered video from a black box."""
        raw = subprocess.run([ADB, '-s', self.s, 'exec-out', 'screencap'], capture_output=True).stdout
        w, h = struct.unpack('<II', raw[:8])
        off = len(raw) - w * h * 4
        out = []
        for x, y in points:
            i = off + (y * w + x) * 4
            out.append(max(raw[i], raw[i + 1], raw[i + 2]))
        return out

    def allow(self):
        """Answers system dialogs: runtime permissions, notifications, media projection
        (Android 14+: "Share your screen with Ami?" with a dropdown and Next)."""
        if self.find('Share one app', exact=True, timeout=1):
            self.tap_text('Share one app', exact=True)
            self.tap_text('Share entire screen', exact=True)
        labels = ('While using the app', 'Allow', 'Start now', 'Start', 'Next') + (('Share screen',) if self.find('Share your screen', timeout=1) else ())
        for label in labels:
            p = self.find(label, exact=True, timeout=1)
            if p:
                self.tap(p)
                return label
        return None

    def restart(self):
        """Cold-starts the app, dismissing dialogs an earlier run may have left behind."""
        for _ in range(3):
            p = self.find('Share entire screen', exact=True, timeout=1) or self.find('Cancel', exact=True, timeout=1)
            if not p:
                break
            self.tap(p)
        self.sh('shell', 'am', 'force-stop', PACKAGE)
        time.sleep(1)
        for _ in range(3):  # make sure the app really came to the front
            self.sh('shell', 'am', 'start', '-n', f'{PACKAGE}/.MainActivity')
            time.sleep(5)
            top = self.sh('shell', 'dumpsys', 'activity', 'activities')
            if 'topResumedActivity' in top and PACKAGE in top.split('topResumedActivity', 1)[1][:200]:
                break

    def login_as(self, user):
        """Signs in as a demo user (logging the current one out first); waits for Metro to bundle."""
        end = time.time() + 150
        while time.time() < end:
            self.allow()
            if self.find('Username', exact=True, timeout=2):
                break
            if self.find("What's on your mind?", timeout=2):
                self.tap_text('Me', exact=True)
                p = self.find('Log out', timeout=10)
                if p:
                    self.tap(p)
                break
        self.tap(self.find('Username', exact=True, timeout=60))
        self.type_text(user)
        self.tap(self.find('Password', exact=True))
        self.type_text(PASSWORD)
        self.hide_kb()
        self.tap_text('Log in', exact=True)
        end = time.time() + 60
        while time.time() < end:
            self.allow()
            if self.find("What's on your mind?", timeout=3):
                return True
        return False


class WS:
    """A bare WebSocket client for the server (stdlib only), enough to join calls."""

    def __init__(self, token):
        host, port = BASE.split('//', 1)[1].split(':')
        self.s = socket.create_connection((host, int(port)))
        self.s.settimeout(8)
        key = base64.b64encode(os.urandom(16)).decode()
        self.s.send(f'GET /ws HTTP/1.1\r\nHost: {host}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Protocol: ami, {token}\r\n\r\n'.encode())
        resp = b''
        while b'\r\n\r\n' not in resp:
            resp += self.s.recv(1024)
        head, self.buf = resp.split(b'\r\n\r\n', 1)
        assert b' 101 ' in head.split(b'\r\n')[0], head

    def _frame(self, op, payload):
        mask = os.urandom(4)
        n = len(payload)
        hdr = bytes([0x80 | op])
        if n < 126:
            hdr += bytes([0x80 | n])
        else:
            hdr += bytes([0x80 | 126]) + struct.pack('>H', n)
        self.s.send(hdr + mask + bytes(b ^ mask[i % 4] for i, b in enumerate(payload)))

    def send(self, obj):
        self._frame(0x1, json.dumps(obj).encode())

    def _read(self, n):
        while len(self.buf) < n:
            c = self.s.recv(4096)
            if not c:
                raise EOFError
            self.buf += c
        d, self.buf = self.buf[:n], self.buf[n:]
        return d

    def recv(self):
        while True:
            h = self._read(2)
            op = h[0] & 0xF
            n = h[1] & 0x7F
            if n == 126:
                n = struct.unpack('>H', self._read(2))[0]
            elif n == 127:
                n = struct.unpack('>Q', self._read(8))[0]
            data = self._read(n)
            if op == 0x9:
                self._frame(0xA, data)
                continue
            if op == 0x8:
                raise EOFError('closed')
            if op == 0x1:
                return json.loads(data)

    def expect(self, typ, timeout=10):
        end = time.time() + timeout
        while time.time() < end:
            e = self.recv()
            if e.get('type') == typ:
                return e
        raise AssertionError('no ' + typ)

    def close(self):
        self.s.close()
