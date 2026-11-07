package main

// This file holds the preload script: the flags and window chrome the page
// expects from a native shell.
//
// The page reaches Go through window.mygo.call('NativeBridge.Dispatch', ...),
// which MyGo defines at document start (see sendNativeBridge in index.html), so
// no bridge shim is injected here. An earlier shim faked
// window.webkit.messageHandlers.moleNative, but WebKit replaces that object when
// the load finishes and silently drops what was attached, which needed a 500ms
// polling watchdog; window.mygo is not replaced, so the page uses it directly.
//
// Everything the old shell injected from AppKit — the isNativeMacApp flag, the
// hidden fake traffic lights, the menubar-mode class — happens here, so the
// page sees the same environment it always did.

// preloadScript returns the JavaScript injected into the windows that show the
// app's page.
//
// The flag arguments pick the two roles the Swift app had:
//
//   - menubar builds the menu-bar popover, which set isMenuBarPopover and added
//     the menubar-mode class; the main window hid the decorative traffic lights
//     and set isNativeMacApp only.
//   - both set isNativeMacApp, which is how the page decides to trust the data
//     the shell pushes instead of the sample data it falls back to in a browser.
func preloadScript(menubar bool) string {
	if menubar {
		return `(function () {
  window.isNativeMacApp = true;
  window.isMenuBarPopover = true;
  onReady(function () {
    document.body.classList.add('menubar-mode');
    installDragRegions();
  });
  function onReady(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn);
    } else {
      fn();
    }
  }
  function installDragRegions() {` + dragRegions + `}
})();`
	}
	return `(function () {
  window.isNativeMacApp = true;
  onReady(function () {
    var lights = document.querySelector('.traffic-lights');
    if (lights) lights.style.visibility = 'hidden';
    installDragRegions();
  });
  function onReady(fn) {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', fn);
    } else {
      fn();
    }
  }
  function installDragRegions() {` + dragRegions + `}
})();`
}

// dragRegions marks the window chrome as a native drag region. MyGo's bridge
// turns a mousedown inside one into a real window drag, which is how the old
// shell answered the page's startWindowDrag and toggleWindowZoom actions.
//
// --app-region is a custom property, so it inherits: marking the two bars is
// enough for their text and icons. The controls inside the header opt back out,
// as the page's own handlers did by stopping propagation.
const dragRegions = `
    var style = document.createElement('style');
    style.textContent = [
      '.window-bar, .top-header { --app-region: drag; -webkit-app-region: drag; }',
      '.header-controls, .traffic-lights, .window-bar button, .top-header button,' +
        ' .top-header input, .top-header select, .top-header a[href],' +
        ' .top-header .btn, .top-header .search-box, .top-header .kbd-tag' +
        ' { --app-region: no-drag; -webkit-app-region: no-drag; }'
    ].join('\n');
    document.head.appendChild(style);
`
