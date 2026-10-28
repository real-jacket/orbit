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
    // The page talks to the shell through window.webkit.messageHandlers.moleNative.
    // Two ordering hazards make a one-shot assignment unreliable here:
    //   * the preload can run before MyGo's bridge script defines window.webkit,
    //     and then "window.webkit.messageHandlers" throws — an assignment wrapped
    //     in try/catch silently leaves the page deaf;
    //   * window.mygo can be installed after the preload ran, so capturing the
    //     runtime in a closure binds undefined.
    // Both were invisible: the page's sendNativeBridge swallows errors, so every
    // action (清理、还原、体检、空间分析) was dropped while Go→page pushes kept
    // working. Retry the attachment and resolve the runtime at call time instead.
    function attachMoleNative(attempt) {
      var handlers = null;
      try { handlers = window.webkit && window.webkit.messageHandlers; } catch (e) {}
      window.__moleDiag = { attempts: attempt, hasWebkit: !!window.webkit, hasHandlers: !!handlers, attached: !!(window.__moleDiag && window.__moleDiag.attached) };
      if (handlers) {
        try {
          handlers.moleNative = {
            postMessage: function (payload) {
              var runtime = window.mygo;
              if (!runtime || typeof runtime.call !== 'function') return;
              try {
                var pending = runtime.call('NativeBridge.Dispatch', payload);
                if (pending && typeof pending.catch === 'function') pending.catch(function () {});
              } catch (e) {}
            }
          };
          if (handlers.moleNative) {
            window.__moleDiag.attached = true;
            return;
          }
        } catch (e) {}
      }
      // 页面脚本一加载就可能发请求，前几秒用更密的间隔补挂
      if (attempt < 400) window.setTimeout(function () { attachMoleNative(attempt + 1); }, attempt < 40 ? 20 : 250);
    }
    // WebKit materialises the real messageHandlers object when the load
    // finishes and silently drops properties added before that, so the
    // attachment has to be re-established when it goes missing. A page cannot
    // tell the difference between "no handler" and "handler not yet attached":
    // both just do nothing, which is how every action after boot disappeared.
    function keepMoleNativeAttached() {
      var handlers = null;
      try { handlers = window.webkit && window.webkit.messageHandlers; } catch (e) {}
      if (handlers && !handlers.moleNative) attachMoleNative(0);
    }
    attachMoleNative(0);
    window.setInterval(keepMoleNativeAttached, 500);
    window.addEventListener('load', keepMoleNativeAttached);
    document.addEventListener('DOMContentLoaded', keepMoleNativeAttached);
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
