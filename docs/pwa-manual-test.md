# Manual PWA Lifecycle Verification

Use `https://` or `http://localhost` so service workers are allowed.

## Desktop Chromium (Chrome/Edge)

- Open `/config` and confirm the PWA status text appears below the control buttons.
- If install prompt is available, click `Install` and verify browser install prompt appears.
- Accept install and confirm status updates to install accepted.
- Click `Reinstall` and verify status reports Sharm service worker unregister/register success.
- Click `Clear local PWA data` and verify status reports removed Sharm service workers/cache buckets.
- Open DevTools Application tab and confirm only Sharm-owned entries are removed.
- If you have unrelated same-origin entries, confirm they remain untouched.
- Confirm app may still appear in OS launcher until manually uninstalled.

## Android Chrome

- Open `/config` in Chrome on Android and wait for page load to settle.
- Tap `Install`; if prompt is unavailable, verify status explains to use browser menu.
- When prompt is shown, complete install and confirm app icon appears on launcher.
- Return to `/config` and run `Reinstall`; verify status reports successful re-registration.
- Run `Clear local PWA data`; verify status reports local cleanup and manual OS uninstall requirement.
- Remove app from launcher/system UI manually and confirm final uninstall behavior.
