import { createLedger } from "./api.js";
import { initCreatePage } from "./create.js";
import { initSharePage } from "./share.js";

const pages = {
  create: initCreatePage,
  share: initSharePage
};

const root = document.querySelector("[data-page]");
const initPage = pages[root?.dataset.page];

if (initPage) {
  root.querySelector("[data-http-warning]").hidden = window.location.protocol !== "http:";

  // The page itself was the first request. The URL fragment is never part of it.
  const ledger = createLedger(root);
  const note = window.location.hash ? "Nothing but the address. The #fragment stayed in this browser." : undefined;
  ledger.record("GET", window.location.pathname, { note })("200 · this page");

  initPage(root, ledger);
}
