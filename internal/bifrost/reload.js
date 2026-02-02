(function () {
  var es = new EventSource("/__bifrost_reload");
  es.addEventListener("reload", function () {
    window.location.reload();
  });
})();
