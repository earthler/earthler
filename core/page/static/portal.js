// The sheet of the apps to add, on the portal. Without this script the
// button still opens it, by its address, and the page works the same.
(function () {
  var sheet = document.getElementById("add-apps");
  var open = document.getElementById("add-open");
  if (!sheet || !open || typeof sheet.showModal !== "function") return;
  // Opened by its address, before the script ran or from a link: the
  // address is cleared, so the sheet no longer shows by it, and the sheet
  // opens as the button opens it, so it closes the same way.
  if (location.hash === "#add-apps") {
    location.replace("#");
    sheet.showModal();
  }
  open.addEventListener("click", function (e) {
    e.preventDefault();
    sheet.showModal();
  });
  // The cross closes the sheet. If the sheet showed by its address, the
  // link itself takes the address away.
  sheet.querySelector("[data-close]").addEventListener("click", function (e) {
    if (!sheet.open) return;
    e.preventDefault();
    sheet.close();
  });
  // A tap on the veil, outside the sheet, closes it too. The veil belongs to
  // the sheet, so the place of the tap tells them apart.
  sheet.addEventListener("click", function (e) {
    if (e.target !== sheet) return;
    var r = sheet.getBoundingClientRect();
    if (e.clientY < r.top || e.clientY > r.bottom || e.clientX < r.left || e.clientX > r.right) sheet.close();
  });
})();
