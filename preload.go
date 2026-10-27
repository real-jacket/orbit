package main

// This file holds the preload script, the one seam that lets the frontend stay
// byte-identical to the one the Swift app shipped.
//
// Resources/src-ui/index.html talks to its shell through WebKit's own message
// handler, which the AppKit app registered as "moleNative":
//
//	window.webkit.messageHandlers.moleNative.postMessage(payload)
//
// MyGo owns the web view instead and registers its handler as "mygo", so that
// name is taken. A preload script runs after MyGo's bridge installs window.mygo
// and before the page's own scripts, which is early enough to add a second
// entry to the same messageHandlers object. Assigning a NEW property there
// works; replacing window.webkit does not, since it is non-configurable.
//
// Everything the old shell injected from AppKit — the isNativeMacApp flag, the
// hidden fake traffic lights, the menubar-mode class — happens here too, so the
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
  var runtime = window.mygo;
  window.isNativeMacApp = true;
  window.isMenuBarPopover = true;
  installNativeBridge(runtime);
  onReady(function () {
    document.body.classList.add('menubar-mode');
    installDragRegions();
  });
  function installNativeBridge(runtime) {` + nativeBridgeShim + `}
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
  var runtime = window.mygo;
  window.isNativeMacApp = true;
  installNativeBridge(runtime);
  onReady(function () {
    var lights = document.querySelector('.traffic-lights');
    if (lights) lights.style.visibility = 'hidden';
    installDragRegions();
  });
  function installNativeBridge(runtime) {` + nativeBridgeShim + `}
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

// nativeBridgeShim defines window.webkit.messageHandlers.moleNative as a
// forwarder into the one bound Go method that handles every action. It is a
// function body so both preload variants can share it.
const nativeBridgeShim = `
    try {
      window.webkit.messageHandlers.moleNative = {
        postMessage: function (payload) {
          if (!runtime || typeof runtime.call !== 'function') return;
          try {
            var pending = runtime.call('NativeBridge.Dispatch', payload);
            if (pending && typeof pending.catch === 'function') pending.catch(function () {});
          } catch (e) {}
        }
      };
    } catch (e) {}
`

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
