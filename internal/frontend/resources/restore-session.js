(function () {
    const clientIdCookieName = "client-id";
    const clientIdStorageKey = "scribble.client-id";
    const clientIdRestoreAttempted = "client_id_restore_attempted";

    function getCookie(name) {
        let cookie = {};
        document.cookie.split(";").forEach(function (el) {
            let split = el.split("=");
            cookie[split[0].trim()] = split.slice(1).join("=");
        });
        return cookie[name];
    }

    function setCookie(name, value, maxAgeSeconds) {
        let cookie = `${name}=${encodeURIComponent(value)}; path=/; SameSite=Strict`;
        if (maxAgeSeconds !== undefined) {
            cookie += `; Max-Age=${maxAgeSeconds}`;
        }
        if (window.location.protocol === "https:") {
            cookie += "; Secure";
        }
        document.cookie = cookie;
    }

    const redirectURL = new URL(window.location.href);
    redirectURL.searchParams.set(clientIdRestoreAttempted, "1");

    try {
        const storedClientId = localStorage.getItem(clientIdStorageKey);
        if (storedClientId) {
            setCookie(clientIdCookieName, storedClientId, 365 * 24 * 60 * 60);
            if (getCookie(clientIdCookieName) === encodeURIComponent(storedClientId)) {
                redirectURL.searchParams.delete(clientIdRestoreAttempted);
            }
        }
    } catch (error) {
        console.error("error restoring client id cookie", error);
    }

    window.location.replace(redirectURL.toString());
})();
