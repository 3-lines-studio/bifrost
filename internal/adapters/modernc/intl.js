// Minimal Intl polyfill for in-process JS runtimes without native Intl.
//
// sobek (the goja fork) and the pure-Go quickjs port have no Intl
// implementation. This shim covers the
// subsets actually exercised by the SSR bundle: dayjs-timezone offset math
// (DateTimeFormat.formatToParts + timeZoneName), date-fns locale formatting
// (DateTimeFormat.format), dayjs locale digit maps (NumberFormat.format),
// and the platform's currency formatting (NumberFormat style: currency).
//
// Timezone handling is intentionally limited: "UTC" is exact, any other
// requested zone falls back to the process-local offset (UTC in the
// deployment container). Nothing in the SSR bundle requests a non-UTC zone.

(function () {
  'use strict';

  var MONTHS_LONG = {
    en: ['January', 'February', 'March', 'April', 'May', 'June', 'July', 'August', 'September', 'October', 'November', 'December'],
    es: ['Enero', 'Febrero', 'Marzo', 'Abril', 'Mayo', 'Junio', 'Julio', 'Agosto', 'Septiembre', 'Octubre', 'Noviembre', 'Diciembre'],
  };
  var MONTHS_SHORT = {
    en: ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'],
    es: ['ene', 'feb', 'mar', 'abr', 'may', 'jun', 'jul', 'ago', 'sep', 'oct', 'nov', 'dic'],
  };
  var WEEKDAYS_LONG = {
    en: ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
    es: ['Domingo', 'Lunes', 'Martes', 'Miércoles', 'Jueves', 'Viernes', 'Sábado'],
  };
  var WEEKDAYS_SHORT = {
    en: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'],
    es: ['dom', 'lun', 'mar', 'mié', 'jue', 'vie', 'sáb'],
  };
  var CURRENCY = { ARS: '$', USD: '$', EUR: '€', GBP: '£', BRL: 'R$', MXN: '$', CLP: '$', UYU: '$U' };

  function localeInfo(locale) {
    var lang = String(locale || 'en-US').split('-')[0].toLowerCase();
    return {
      lang: lang,
      group: lang === 'es' ? '.' : ',',
      decimal: lang === 'es' ? ',' : '.',
    };
  }

  function pad2(n) {
    return n < 10 ? '0' + n : String(n);
  }

  // Offset of the process-local timezone in minutes (UTC+ is negative, like
  // Date.getTimezoneOffset). The deployment container runs UTC.
  function localOffset() {
    return new Date().getTimezoneOffset();
  }

  function gmtString(offsetMin) {
    if (offsetMin === 0) return 'GMT';
    var sign = offsetMin > 0 ? '-' : '+';
    var abs = Math.abs(offsetMin);
    var h = Math.floor(abs / 60);
    var m = abs % 60;
    return 'GMT' + sign + pad2(h) + (m !== 0 ? ':' + pad2(m) : '');
  }

  function timeZoneNameFor(zone, style) {
    var offsetMin = zone === 'UTC' ? 0 : localOffset();
    if (offsetMin === 0) return 'UTC';
    if (style === 'long' || style === 'longOffset' || style === 'shortOffset') return gmtString(offsetMin);
    return gmtString(offsetMin).replace('GMT', 'GMT');
  }

  function zoneShift(zone) {
    return zone === 'UTC' ? 0 : localOffset();
  }

  function components(date, zone) {
    var shift = zoneShift(zone);
    var d = new Date(date.getTime() - shift * 60000);
    return {
      year: d.getUTCFullYear(),
      month: d.getUTCMonth(),
      day: d.getUTCDate(),
      hour: d.getUTCHours(),
      minute: d.getUTCMinutes(),
      second: d.getUTCSeconds(),
      weekday: d.getUTCDay(),
      zone: zone,
    };
  }

  function monthName(c, width, lang) {
    var table = width === 'long' ? MONTHS_LONG : MONTHS_SHORT;
    var list = table[lang] || table.en;
    return list[c.month];
  }

  function weekdayName(c, width, lang) {
    var table = width === 'long' ? WEEKDAYS_LONG : WEEKDAYS_SHORT;
    var list = table[lang] || table.en;
    return list[c.weekday];
  }

  function hour12(hour, hour12Flag) {
    if (hour12Flag === false) return { h: hour, suffix: '' };
    var h = hour % 12;
    return { h: h === 0 ? 12 : h, suffix: hour < 12 ? 'AM' : 'PM' };
  }

  class DateTimeFormat {
    constructor(locale, options) {
      this.locale = String(locale || 'en-US');
      this.options = options || {};
      this.timeZone = this.options.timeZone || 'UTC';
      this.info = localeInfo(this.locale);
    }

    resolvedOptions() {
      var o = this.options;
      var out = { locale: this.locale, timeZone: this.timeZone, calendar: 'gregory', numberingSystem: 'latn' };
      if (o.hour12 !== undefined) out.hour12 = o.hour12;
      return out;
    }

    formatToParts(date) {
      var c = components(toDate(date), this.timeZone);
      var o = this.options;
      var parts = [];
      if (o.weekday) parts.push({ type: 'weekday', value: weekdayName(c, o.weekday, this.info.lang) });
      if (o.year) parts.push({ type: 'year', value: o.year === '2-digit' ? pad2(c.year % 100) : String(c.year) });
      if (o.month) parts.push({ type: 'month', value: o.month === 'numeric' ? String(c.month + 1) : o.month === '2-digit' ? pad2(c.month + 1) : monthName(c, o.month, this.info.lang) });
      if (o.day) parts.push({ type: 'day', value: o.day === '2-digit' ? pad2(c.day) : String(c.day) });
      var h12 = hour12(c.hour, o.hour12);
      if (o.hour) parts.push({ type: 'hour', value: o.hour === '2-digit' ? pad2(h12.h) : String(h12.h) });
      if (o.minute) parts.push({ type: 'minute', value: o.minute === '2-digit' ? pad2(c.minute) : String(c.minute) });
      if (o.second) parts.push({ type: 'second', value: o.second === '2-digit' ? pad2(c.second) : String(c.second) });
      if (o.hour12 === true && o.hour) parts.push({ type: 'dayPeriod', value: h12.suffix });
      if (o.timeZoneName) parts.push({ type: 'timeZoneName', value: timeZoneNameFor(this.timeZone, o.timeZoneName) });
      return parts;
    }

    format(date) {
      var parts = this.formatToParts(date);
      var o = this.options;
      if (o.hour && !o.year && !o.month && !o.day) {
        var time = parts.filter(function (p) { return p.type === 'hour' || p.type === 'minute' || p.type === 'second'; })
          .map(function (p) { return p.value; }).join(':');
        var period = '';
        for (var i = 0; i < parts.length; i++) if (parts[i].type === 'dayPeriod') period = ' ' + parts[i].value;
        var zone = '';
        for (var j = 0; j < parts.length; j++) if (parts[j].type === 'timeZoneName') zone = ' ' + parts[j].value;
        return time + period + zone;
      }
      if (o.month === 'long' || o.month === 'short') {
        var month = '';
        var year = '';
        var day = '';
        for (var k = 0; k < parts.length; k++) {
          if (parts[k].type === 'month') month = parts[k].value;
          if (parts[k].type === 'year') year = parts[k].value;
          if (parts[k].type === 'day') day = parts[k].value;
        }
        var joined = year ? month + ' ' + year : month;
        return day ? month + ' ' + day + (year ? ', ' + year : '') : joined;
      }
      var dateOnly = parts.filter(function (p) { return p.type === 'month' || p.type === 'day' || p.type === 'year'; })
        .map(function (p) { return p.value; });
      if (o.month === 'numeric' || o.month === '2-digit' || (o.month === undefined && o.day)) {
        var d = parts.filter(function (p) { return p.type === 'day'; })[0];
        var m = parts.filter(function (p) { return p.type === 'month'; })[0];
        var y = parts.filter(function (p) { return p.type === 'year'; })[0];
        var es = this.info.lang === 'es';
        var a = [m ? m.value : '1', d ? d.value : '1', y ? y.value : String(c.year)].map(function (x) { return x; });
        return es ? a[1] + '/' + a[0] + '/' + a[2] : a[0] + '/' + a[1] + '/' + a[2];
      }
      return dateOnly.join('/');
    }
  }

  DateTimeFormat.supportedLocalesOf = function (locales) { return Array.isArray(locales) ? locales.slice() : [String(locales)]; };

  class NumberFormat {
    constructor(locale, options) {
      this.locale = String(locale || 'en-US');
      this.options = options || {};
      this.info = localeInfo(this.locale);
      this.minFrac = this.options.minimumFractionDigits;
      if (this.minFrac === undefined) this.minFrac = this.options.style === 'currency' ? 2 : 0;
      this.maxFrac = this.options.maximumFractionDigits !== undefined ? this.options.maximumFractionDigits : this.minFrac;
      this.symbol = this.options.currency ? (CURRENCY[this.options.currency] || this.options.currency + ' ') : '';
    }

    resolvedOptions() {
      var out = { locale: this.locale, numberingSystem: this.options.numberingSystem || 'latn', style: this.options.style || 'decimal', minimumFractionDigits: this.minFrac, maximumFractionDigits: this.maxFrac };
      if (this.options.currency) out.currency = this.options.currency;
      return out;
    }

    format(value) {
      var n = Number(value);
      if (!isFinite(n)) return String(value);
      var neg = n < 0;
      var fixed = Math.abs(n).toFixed(this.maxFrac);
      var dot = fixed.indexOf('.');
      var intPart = dot === -1 ? fixed : fixed.slice(0, dot);
      var fracPart = dot === -1 ? '' : fixed.slice(dot + 1);
      while (fracPart.length > this.minFrac && fracPart.charAt(fracPart.length - 1) === '0') {
        fracPart = fracPart.slice(0, -1);
      }
      var grouped = intPart.replace(/\B(?=(\d{3})+(?!\d))/g, this.info.group);
      var out = grouped + (fracPart !== '' ? this.info.decimal + fracPart : '');
      if (this.options.style === 'currency') out = this.symbol + out;
      return neg ? '-' + out : out;
    }

    formatToParts(value) {
      var out = [];
      var formatted = this.format(value);
      if (this.options.style === 'currency') out.push({ type: 'currency', value: this.symbol });
      var m = formatted.match(/(\d+)([.,](\d+))?/);
      if (m) {
        out.push({ type: 'integer', value: m[1].replace(/[^0-9]/g, '') });
        if (m[3] !== undefined) out.push({ type: 'fraction', value: m[3] });
      }
      return out;
    }
  }

  NumberFormat.supportedLocalesOf = function (locales) { return Array.isArray(locales) ? locales.slice() : [String(locales)]; };

  class Collator {
    constructor(locale, options) {
      this.locale = String(locale || 'en-US');
      this.options = options || {};
    }
    compare(a, b) {
      a = String(a);
      b = String(b);
      return a < b ? -1 : a > b ? 1 : 0;
    }
    resolvedOptions() {
      return { locale: this.locale, sensitivity: this.options.sensitivity || 'variant' };
    }
  }

  Collator.supportedLocalesOf = function (locales) { return Array.isArray(locales) ? locales.slice() : [String(locales)]; };

  class RelativeTimeFormat {
    constructor(locale, options) {
      this.locale = String(locale || 'en-US');
      this.options = options || {};
    }
    format(value, unit) {
      return String(value) + ' ' + unit;
    }
    resolvedOptions() {
      return { locale: this.locale, numeric: this.options.numeric || 'always', style: this.options.style || 'long' };
    }
  }

  function toDate(value) {
    if (value instanceof Date) return value;
    return new Date(value);
  }

  globalThis.Intl = {
    DateTimeFormat: DateTimeFormat,
    NumberFormat: NumberFormat,
    Collator: Collator,
    RelativeTimeFormat: RelativeTimeFormat,
  };
})();
