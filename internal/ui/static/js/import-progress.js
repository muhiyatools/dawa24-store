/*
 * One progress bar, shared by every import tool.
 *
 * The four importers each grew their own bar and each got the same three things
 * wrong. This is the single implementation they now use.
 *
 * What it fixes:
 *
 *  1. The number is always shown. A bar with no percentage answers "is it
 *     working?" and never "how much longer?", which is the only question the
 *     person watching actually has.
 *
 *  2. It keeps moving while the server is silent. A poll that returns the same
 *     percentage for ninety seconds — an AI adjudication pass, a large commit —
 *     used to freeze the bar, which reads as a hung import. Between server
 *     updates the bar drifts forward on an easing curve that approaches the
 *     next server value without ever passing it, so motion means "still
 *     working" and never invents progress the server has not reported.
 *
 *  3. It never reaches 100 before the work does. Every "stuck at 100%" report
 *     came from a bar that hit the end when a background call returned, while
 *     the commit that followed was still writing. Display is capped at 99 until
 *     the server says done.
 *
 * It also never goes backwards: a server that re-reports a lower percentage
 * (a retried stage, a phase boundary computed differently) holds the bar where
 * it is rather than rewinding it, because a bar that goes back is read as a
 * failure even when the run is healthy.
 */
(function () {
	'use strict';

	// How close the drift may creep toward the next known target, as a share of
	// the remaining gap. Short of 1 so the bar is always visibly behind what the
	// server has confirmed.
	var DRIFT_CEILING = 0.9;
	// Seconds for the drift to cover half its allowed gap.
	var DRIFT_HALF_LIFE = 8;
	// The display cap before completion. See point 3 above.
	var MAX_BEFORE_DONE = 99;

	function clamp(v, lo, hi) {
		return Math.max(lo, Math.min(hi, v));
	}

	/**
	 * ImportProgress renders one bar and keeps it honest.
	 *
	 * opts: { fill, label, percent, count, onDone }
	 *   fill    element whose width is the bar
	 *   label   element for the phase message
	 *   percent element for the numeric percentage
	 *   count   element for "1,204 / 9,000"
	 *   onDone  called once, when the server reports completion
	 */
	function ImportProgress(opts) {
		this.fill = opts.fill || null;
		this.labelEl = opts.label || null;
		this.percentEl = opts.percent || null;
		this.countEl = opts.count || null;
		this.onDone = opts.onDone || function () {};

		// shown is what the user sees; target is the last value the server
		// confirmed. shown drifts toward target and stops short of it.
		this.shown = 0;
		this.target = 0;
		this.done = false;
		this.lastServerAt = Date.now();
		this.timer = null;
		this.render();
	}

	ImportProgress.prototype.render = function () {
		var v = this.done ? 100 : Math.floor(clamp(this.shown, 0, MAX_BEFORE_DONE));
		if (this.fill) {
			this.fill.style.width = v + '%';
			this.fill.classList.remove('is-indeterminate');
			this.fill.setAttribute('aria-valuenow', String(v));
		}
		if (this.percentEl) {
			this.percentEl.textContent = v + '%';
		}
	};

	/** update applies a server snapshot. */
	ImportProgress.prototype.update = function (snapshot) {
		if (!snapshot) return;
		if (typeof snapshot.percent === 'number' && snapshot.percent >= 0) {
			// Never rewind. A lower number from the server is a recomputation,
			// not a regression in the work.
			this.target = Math.max(this.target, snapshot.percent);
			this.shown = Math.max(this.shown, Math.min(this.shown, this.target));
			this.lastServerAt = Date.now();
		}
		if (this.labelEl && snapshot.message) {
			this.labelEl.textContent = snapshot.message;
		}
		if (this.countEl) {
			if (snapshot.total > 0) {
				this.countEl.textContent =
					Number(snapshot.current || 0).toLocaleString('en-US') +
					' / ' +
					Number(snapshot.total).toLocaleString('en-US');
			} else {
				this.countEl.textContent = '';
			}
		}
		if (snapshot.done) {
			this.finish();
			return;
		}
		this.render();
	};

	/** finish is the only path to 100. */
	ImportProgress.prototype.finish = function () {
		if (this.done) return;
		this.done = true;
		this.shown = 100;
		this.stop();
		this.render();
		this.onDone();
	};

	/** fail leaves the bar where it stopped, so the reader can see how far it got. */
	ImportProgress.prototype.fail = function (message) {
		this.stop();
		if (this.fill) this.fill.classList.add('is-failed');
		if (this.labelEl && message) this.labelEl.textContent = message;
		this.render();
	};

	/** start begins the drift animation. */
	ImportProgress.prototype.start = function () {
		if (this.timer) return;
		var self = this;
		this.timer = setInterval(function () {
			if (self.done) return;
			var idle = (Date.now() - self.lastServerAt) / 1000;
			var gap = self.target - self.shown;
			if (gap > 0.2) {
				// Ease toward what the server already confirmed.
				self.shown += gap * 0.18;
			} else if (idle > 1) {
				// The server has told us nothing new. Creep toward the next
				// whole point without ever arriving, so the bar shows life
				// without claiming progress nobody reported.
				var room = (self.target + 1) * DRIFT_CEILING - self.shown;
				if (room > 0) {
					self.shown += room * (1 - Math.pow(0.5, 0.25 / DRIFT_HALF_LIFE));
				}
			}
			self.render();
		}, 250);
	};

	ImportProgress.prototype.stop = function () {
		if (this.timer) {
			clearInterval(this.timer);
			this.timer = null;
		}
	};

	/**
	 * poll drives the bar from a JSON endpoint until it reports done.
	 *
	 * A failed poll is not a failed import: networks blink and a background run
	 * outlives the page watching it. Failures are counted, and only a sustained
	 * run of them stops the bar.
	 */
	ImportProgress.prototype.poll = function (url, intervalMs) {
		var self = this;
		var failures = 0;
		this.start();
		var tick = function () {
			fetch(url, { headers: { Accept: 'application/json' } })
				.then(function (res) {
					if (!res.ok) throw new Error('HTTP ' + res.status);
					return res.json();
				})
				.then(function (data) {
					failures = 0;
					self.update(data);
					if (!self.done) setTimeout(tick, intervalMs || 1500);
				})
				.catch(function () {
					failures++;
					if (failures >= 8) {
						self.fail('تعذّر الاتصال بالخادم لمتابعة التقدم. المعالجة قد تكون مستمرة في الخلفية — أعد تحميل الصفحة.');
						return;
					}
					setTimeout(tick, (intervalMs || 1500) * 2);
				});
		};
		tick();
	};

	window.ImportProgress = ImportProgress;
})();
