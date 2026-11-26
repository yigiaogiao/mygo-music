// The LazyCat Android client gives pages a native media session as
// window.lzc_media_session. It is shaped unlike the browser's Media
// Session API: metadata and position go in as JSON, and actions come back
// as "lzc_media_session_event" events rather than to the handlers set for
// them. MyGo asks for the browser's API, and a WebView that is not a
// secure context has none, so wrap the client's in one: the notification
// and the lock screen then show the song, with play, pause, next, previous
// and a seek bar.
//
// In a browser, and wherever the client gives no bridge, this does nothing.
(function () {
  'use strict';

  var bridge = window.lzc_media_session;
  if (!bridge) return;

  // MyGo builds a MediaMetadata, which a WebView with no standard API has
  // not.
  if (typeof window.MediaMetadata !== 'function') {
    window.MediaMetadata = function (init) {
      if (!init) return;
      for (var k in init) {
        if (Object.prototype.hasOwnProperty.call(init, k)) this[k] = init[k];
      }
    };
  }

  var handlers = Object.create(null);
  // sessionDuration is what the position state last said, in seconds: a
  // seek beyond it came in milliseconds, as some clients send it.
  var sessionDuration = 0;

  // seekTime reads where a seek goes, in seconds. The client sends the
  // session's times, which the position state also gives, but the value
  // comes as a number, a string, or an object, so take any of them.
  // null says the event carries no time: seeking nowhere must not reach
  // the player, which reads the time as a number and would crash.
  function seekTime(data, asSeconds) {
    var t = data;
    if (typeof t === 'string') {
      var s = t.trim();
      if (s.charAt(0) === '{') {
        try {
          t = JSON.parse(s);
        } catch (err) {
          return null;
        }
      } else {
        t = Number(s);
      }
    }
    if (t && typeof t === 'object') {
      if (t.seekTime != null) t = t.seekTime;
      else if (t.position != null) t = t.position;
      else if (t.seekPosition != null) t = t.seekPosition;
      else if (t.time != null) t = t.time;
    }
    if (typeof t === 'string') t = Number(t);
    if (typeof t !== 'number' || !isFinite(t)) return null;
    // The session's times are seconds. A value of a thousand or more past
    // the track, whose thousandth is within it, is milliseconds: only then
    // is it read so, as a seconds value past the end is just a rounding.
    if (!asSeconds && t >= 1000 && sessionDuration > 0 && t > sessionDuration && t / 1000 <= sessionDuration) {
      t = t / 1000;
    }
    return t < 0 ? 0 : t;
  }

  function dispatch(action, data, fn, asSeconds) {
    var args = { action: action };
    if (action === 'seekto') {
      var t = seekTime(data, asSeconds);
      if (t === null) return;
      args.seekTime = t;
    } else if (data && typeof data === 'object') {
      for (var k in data) {
        if (!(k in args)) args[k] = data[k];
      }
    }
    try {
      fn(args);
    } catch (err) {
      console.error('media session handler', action, err);
    }
  }

  window.addEventListener('lzc_media_session_event', function (e) {
    var detail = (e && e.detail) || {};
    var action = detail.eventType || detail.action;
    var fn = action && handlers[action];
    if (typeof fn !== 'function') return;
    dispatch(action, detail.data, fn, false);
  });

  // LazyCat Music answers the client's media controls with these globals,
  // which the client calls for the lock screen and the notification rather
  // than sending the event. A page that defines them keeps those controls
  // working once the screen is off.
  function nativeAction(action, data, asSeconds) {
    var fn = handlers[action];
    if (typeof fn === 'function') dispatch(action, data, fn, asSeconds);
  }
  window.__nativePlay = function () { nativeAction('play'); };
  window.__nativePause = function () { nativeAction('pause'); };
  window.__nativePrev = function () { nativeAction('previoustrack'); };
  window.__nativeNext = function () { nativeAction('nexttrack'); };
  window.__nativeSeekTo = function (time) { nativeAction('seekto', time, true); };

  function dataURL(src) {
    return fetch(src)
      .then(function (r) { return r.blob(); })
      .then(function (blob) {
        return new Promise(function (resolve, reject) {
          var fr = new FileReader();
          fr.onload = function () { resolve(fr.result); };
          fr.onerror = reject;
          fr.readAsDataURL(blob);
        });
      });
  }

  // artworkFor turns what MyGo has, the cover's bytes as a blob URL, into
  // something the client can load itself: a data URL. Until it has it, the
  // song shows without art.
  function artworkFor(art, done) {
    if (!art || !art.length) return done(undefined);
    var first = art[0];
    var src = first && first.src;
    if (typeof src !== 'string' || src.indexOf('blob:') !== 0) return done(art);
    done(undefined);
    dataURL(src)
      .then(function (url) { done([{ src: url, type: first.type }]); })
      .catch(function () {});
  }

  var session = {
    setActionHandler: function (action, fn) {
      handlers[action] = typeof fn === 'function' ? fn : null;
      // The client shows the action once asked; null leaves ours out of it.
      if (typeof fn === 'function' && typeof bridge.setActionHandler === 'function') {
        try {
          bridge.setActionHandler(action);
        } catch (err) {
          console.warn('media session action', action, err);
        }
      }
    },
    setPositionState: function (state) {
      if (typeof bridge.setPositionState !== 'function') return;
      var s = state || {};
      var out = {};
      if (s.duration > 0) {
        out.duration = s.duration;
        sessionDuration = s.duration;
      }
      if (s.position >= 0) out.position = s.position;
      if (s.playbackRate > 0) out.playbackRate = s.playbackRate;
      try {
        bridge.setPositionState(JSON.stringify(out));
      } catch (err) {
        console.warn('media session position', err);
      }
    }
  };

  Object.defineProperty(session, 'metadata', {
    get: function () { return this._metadata || null; },
    set: function (md) {
      this._metadata = md;
      var out = {};
      if (md) {
        if (md.title) out.title = md.title;
        if (md.artist) out.artist = md.artist;
        if (md.album) out.album = md.album;
      }
      var send = function (artwork) {
        var msg = {};
        for (var k in out) msg[k] = out[k];
        if (artwork) msg.artwork = artwork;
        try {
          bridge.setMetadata(JSON.stringify(msg));
        } catch (err) {
          console.warn('media session metadata', err);
        }
      };
      if (md && md.artwork) artworkFor(md.artwork, send);
      else send();
    }
  });

  Object.defineProperty(session, 'playbackState', {
    get: function () { return this._playbackState || 'none'; },
    set: function (state) {
      this._playbackState = state;
      if (typeof bridge.setPlaybackState !== 'function') return;
      try {
        bridge.setPlaybackState(state);
      } catch (err) {
        console.warn('media session state', err);
      }
    }
  });

  try {
    Object.defineProperty(navigator, 'mediaSession', { value: session, configurable: true });
  } catch (err) {
    if (!navigator.mediaSession) {
      try { navigator.mediaSession = session; } catch (e) { /* it stays the client's */ }
    }
  }
})();
