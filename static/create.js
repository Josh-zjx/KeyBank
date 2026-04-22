(() => {
  const form = document.querySelector("[data-create-form]");
  if (!form) {
    return;
  }

  const presetInputs = Array.from(form.querySelectorAll('input[name="expiryPreset"]'));
  const customInput = document.getElementById("custom-ttl");
  const status = document.getElementById("create-status");
  const result = document.getElementById("create-result");

  function setStatus(message, state) {
    status.textContent = message;
    status.dataset.state = state;
  }

  function syncCustomInput() {
    const customSelected = presetInputs.some((input) => input.checked && input.value === "custom");
    customInput.disabled = !customSelected;
  }

  function selectedTTL() {
    const selected = presetInputs.find((input) => input.checked);
    if (!selected || selected.value === "") {
      return 86400;
    }
    if (selected.value !== "custom") {
      return Number(selected.value);
    }

    const minutes = Number(customInput.value);
    if (!Number.isFinite(minutes) || minutes < 1) {
      throw new Error("Enter a custom expiry of at least 1 minute.");
    }

    const ttl = minutes * 60;
    if (ttl > 604800) {
      throw new Error("Custom expiry must be 7 days or less.");
    }
    return ttl;
  }

  function renderResult(payload) {
    result.hidden = false;
    result.textContent = JSON.stringify(
      {
        id: payload.id,
        pub_pem: payload.pub_pem
      },
      null,
      2
    );
  }

  presetInputs.forEach((input) => input.addEventListener("change", syncCustomInput));
  syncCustomInput();

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    result.hidden = true;
    result.textContent = "";

    let ttl;
    try {
      ttl = selectedTTL();
    } catch (error) {
      const message = error instanceof Error ? error.message : "Enter a valid custom expiry.";
      setStatus(message, "error");
      return;
    }

    setStatus("Requesting a one-time keypair...", "working");

    try {
      const response = await fetch("/api/keys", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ ttl })
      });

      const rawBody = await response.text();
      if (!response.ok) {
        throw new Error(rawBody || "Key creation failed.");
      }

      let payload;
      try {
        payload = JSON.parse(rawBody);
      } catch {
        throw new Error("Key creation API returned invalid JSON.");
      }

      renderResult(payload);
      setStatus(
        "Stub API call complete. The message never left this page; share-link assembly lands in milestone 5.",
        "success"
      );
    } catch (error) {
      const message = error instanceof Error ? error.message : "Key creation failed.";
      setStatus(message, "error");
    }
  });
})();
