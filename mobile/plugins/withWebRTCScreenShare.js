// Expo config plugin: enables react-native-webrtc's media-projection foreground
// service so getDisplayMedia() (screen sharing) works on Android 10+ / 14+.
// The required permissions are declared in app.json ("android.permissions").
const { withMainActivity } = require('expo/config-plugins');

const IMPORT = 'import com.oney.WebRTCModule.WebRTCModuleOptions';
const ENABLE = 'WebRTCModuleOptions.getInstance().enableMediaProjectionService = true';

module.exports = function withWebRTCScreenShare(config) {
  return withMainActivity(config, (c) => {
    let src = c.modResults.contents;
    if (c.modResults.language !== 'kt') {
      throw new Error('withWebRTCScreenShare: only Kotlin MainActivity is supported');
    }
    if (!src.includes(IMPORT)) {
      src = src.replace(/^(package [^\n]+\n)/m, `$1\n${IMPORT}\n`);
    }
    if (!src.includes(ENABLE)) {
      src = src.replace(/(super\.onCreate\([^)]*\)\n)/, `$1    ${ENABLE}\n`);
    }
    c.modResults.contents = src;
    return c;
  });
};
