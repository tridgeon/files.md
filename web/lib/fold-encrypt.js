// HyperMD addon: fold encrypted inline tokens as masked text with click actions.
(function (mod){ //[HyperMD] UMD patched!
    /*commonjs*/  ("object"==typeof exports&&"undefined"!=typeof module) ? mod(null, exports, require("codemirror"), require("../core"), require("./fold")) :
        /*amd*/       ("function"==typeof define&&define.amd) ? define(["require","exports","codemirror","../core","./fold"], mod) :
            /*plain env*/ mod(null, (this.HyperMD.FoldEncrypt = this.HyperMD.FoldEncrypt || {}), CodeMirror, HyperMD, HyperMD.Fold);
})(function (require, exports, CodeMirror, core_1, fold_1) {
    "use strict";
    Object.defineProperty(exports, "__esModule", { value: true });

    var TOKEN_RE = /^!enc\[v1:([0-9]+):([A-Za-z0-9_-]+):([A-Za-z0-9_-]+):([A-Za-z0-9_-]+)\]/;
    var CACHE_TTL_MS = 5 * 60 * 1000;

    var passwordCache = {
        value: null,
        expiresAt: 0,
    };

    function nowMs() {
        return Date.now();
    }

    function setCachedPassword(password) {
        passwordCache.value = password;
        passwordCache.expiresAt = nowMs() + CACHE_TTL_MS;
    }

    function clearCachedPassword() {
        passwordCache.value = null;
        passwordCache.expiresAt = 0;
    }

    function getCachedPassword() {
        if (!passwordCache.value) return null;
        if (nowMs() >= passwordCache.expiresAt) {
            clearCachedPassword();
            return null;
        }
        return passwordCache.value;
    }

    function showToastSafe(message) {
        if (typeof window.showToast === "function") {
            window.showToast(message);
            return;
        }
        var toast = document.createElement("div");
        toast.textContent = message;
        toast.style.cssText = "position:fixed;top:8px;left:50%;transform:translateX(-50%);background:var(--col-bg-alt);color:var(--col-tx);padding:8px 16px;border-radius:5px;border:1px solid var(--col-border);z-index:9999;font-size:14px;";
        document.body.appendChild(toast);
        setTimeout(function () { if (toast.parentNode) toast.parentNode.removeChild(toast); }, 1500);
    }

    function b64ToBytes(base64url) {
        var b64 = base64url.replace(/-/g, "+").replace(/_/g, "/");
        while (b64.length % 4) b64 += "=";
        var raw = atob(b64);
        var arr = new Uint8Array(raw.length);
        for (var i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
        return arr;
    }

    function bytesToB64(bytes) {
        var bin = "";
        for (var i = 0; i < bytes.length; i++) {
            bin += String.fromCharCode(bytes[i]);
        }
        return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
    }

    function parseToken(text) {
        var m = text.match(TOKEN_RE);
        if (!m) return null;
        return {
            raw: m[0],
            iterations: parseInt(m[1], 10),
            salt: m[2],
            iv: m[3],
            data: m[4],
            endCh: m[0].length,
        };
    }

    function parseTokenAt(line, startCh) {
        var sub = line.slice(startCh);
        var parsed = parseToken(sub);
        if (!parsed) return null;
        return {
            raw: parsed.raw,
            iterations: parsed.iterations,
            salt: parsed.salt,
            iv: parsed.iv,
            data: parsed.data,
            fromCh: startCh,
            toCh: startCh + parsed.endCh,
        };
    }

    async function deriveKey(password, saltBytes, iterations) {
        var enc = new TextEncoder();
        var passwordKey = await crypto.subtle.importKey(
            "raw",
            enc.encode(password),
            "PBKDF2",
            false,
            ["deriveKey"]
        );
        return await crypto.subtle.deriveKey(
            {
                name: "PBKDF2",
                salt: saltBytes,
                iterations: iterations,
                hash: "SHA-256",
            },
            passwordKey,
            {
                name: "AES-GCM",
                length: 256,
            },
            false,
            ["encrypt", "decrypt"]
        );
    }

    async function encryptToken(plaintext, password, iterations) {
        if (typeof crypto === "undefined" || !crypto.subtle) {
            throw new Error("WebCrypto is unavailable");
        }
        var enc = new TextEncoder();
        var salt = crypto.getRandomValues(new Uint8Array(16));
        var iv = crypto.getRandomValues(new Uint8Array(12));
        var key = await deriveKey(password, salt, iterations);
        var cipher = await crypto.subtle.encrypt(
            { name: "AES-GCM", iv: iv },
            key,
            enc.encode(plaintext)
        );
        var data = new Uint8Array(cipher);
        return "!enc[v1:" + iterations + ":" + bytesToB64(salt) + ":" + bytesToB64(iv) + ":" + bytesToB64(data) + "]";
    }
    exports.encryptToken = encryptToken;

    async function decryptToken(parsed, password) {
        var salt = b64ToBytes(parsed.salt);
        var iv = b64ToBytes(parsed.iv);
        var data = b64ToBytes(parsed.data);
        var key = await deriveKey(password, salt, parsed.iterations);
        var plainBuf = await crypto.subtle.decrypt(
            { name: "AES-GCM", iv: iv },
            key,
            data
        );
        return new TextDecoder().decode(plainBuf);
    }

    async function copyToClipboard(text) {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            await navigator.clipboard.writeText(text);
            return;
        }
        var ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.left = "-9999px";
        document.body.appendChild(ta);
        ta.select();
        document.execCommand("copy");
        document.body.removeChild(ta);
    }

    function createModal(options) {
        return new Promise(function (resolve) {
            var backdrop = document.createElement("div");
            backdrop.className = "enc-modal-backdrop";

            var modal = document.createElement("div");
            modal.className = "enc-modal";

            var title = document.createElement("h3");
            title.className = "enc-modal-title";
            title.textContent = options.title;

            var input;
            if (options.multiline) {
                input = document.createElement("textarea");
                input.className = "enc-modal-input enc-modal-input-area";
                input.rows = 6;
            } else {
                input = document.createElement("input");
                input.className = "enc-modal-input";
                input.type = options.password ? "password" : "text";
            }
            input.value = options.value || "";
            input.placeholder = options.placeholder || "";

            var actions = document.createElement("div");
            actions.className = "enc-modal-actions";

            var cancelBtn = document.createElement("button");
            cancelBtn.className = "enc-modal-btn";
            cancelBtn.textContent = "Cancel";

            var okBtn = document.createElement("button");
            okBtn.className = "enc-modal-btn enc-modal-btn-primary";
            okBtn.textContent = options.okText || "OK";

            actions.appendChild(cancelBtn);
            actions.appendChild(okBtn);

            modal.appendChild(title);
            modal.appendChild(input);
            modal.appendChild(actions);
            backdrop.appendChild(modal);
            document.body.appendChild(backdrop);

            function finish(val) {
                if (backdrop.parentNode) backdrop.parentNode.removeChild(backdrop);
                document.removeEventListener("keydown", onKeyDown, true);
                resolve(val);
            }

            function onKeyDown(ev) {
                if (ev.key === "Escape") {
                    ev.preventDefault();
                    ev.stopPropagation();
                    finish(null);
                    return;
                }
                if (ev.key === "Enter" && !options.multiline) {
                    ev.preventDefault();
                    ev.stopPropagation();
                    finish(input.value);
                }
            }

            backdrop.addEventListener("click", function (ev) {
                if (ev.target === backdrop) finish(null);
            });
            cancelBtn.addEventListener("click", function () { finish(null); });
            okBtn.addEventListener("click", function () { finish(input.value); });
            document.addEventListener("keydown", onKeyDown, true);

            setTimeout(function () {
                input.focus();
                input.select && input.select();
            }, 0);
        });
    }

    async function getMasterPassword() {
        var cached = getCachedPassword();
        if (cached) return cached;

        var pw = await createModal({
            title: "Enter master password",
            placeholder: "Master password",
            password: true,
            okText: "Unlock",
            multiline: false,
        });
        if (pw === null || pw === "") return null;
        setCachedPassword(pw);
        return pw;
    }

    async function decryptWithPrompt(parsed) {
        var pw = await getMasterPassword();
        if (pw === null) return null;
        try {
            return await decryptToken(parsed, pw);
        } catch (_) {
            clearCachedPassword();
            showToastSafe("Wrong password");
            var retry = await getMasterPassword();
            if (retry === null) return null;
            return await decryptToken(parsed, retry);
        }
    }

    function clearSingleClickTimer(el) {
        if (el._hmdEncryptClickTimer) {
            clearTimeout(el._hmdEncryptClickTimer);
            el._hmdEncryptClickTimer = null;
        }
    }

    exports.EncryptFolder = function (stream, token) {
        if (!token || !token.string || token.string !== "!") return null;

        var cm = stream.cm;
        var lineText = cm.getLine(stream.lineNo);
        var parsed = parseTokenAt(lineText, token.start);
        if (!parsed) return null;

        var from = { line: stream.lineNo, ch: parsed.fromCh };
        var to = { line: stream.lineNo, ch: parsed.toCh };

        var req = stream.requestRange(from, to);
        if (req !== fold_1.RequestRangeResult.OK) return null;

        var addon = exports.getAddon(cm);
        var mask = addon.mask || "****";
        var span = document.createElement("span");
        span.className = "hmd-encrypted-mask";
        span.textContent = mask;
        span.title = "Single click to copy decrypted text. Double click to edit.";

        var marker = cm.markText(from, to, {
            collapsed: true,
            clearOnEnter: false,
            replacedWith: span,
        });

        span.addEventListener("click", function (ev) {
            ev.preventDefault();
            ev.stopPropagation();
            clearSingleClickTimer(span);
            span._hmdEncryptClickTimer = setTimeout(async function () {
                try {
                    var plain = await decryptWithPrompt(parsed);
                    if (plain === null) return;
                    await copyToClipboard(plain);
                    showToastSafe("Copied decrypted text");
                } catch (err) {
                    showToastSafe("Decrypt failed");
                }
            }, 220);
        });

        span.addEventListener("dblclick", function (ev) {
            ev.preventDefault();
            ev.stopPropagation();
            clearSingleClickTimer(span);
            (async function () {
                try {
                    var plain = await decryptWithPrompt(parsed);
                    if (plain === null) return;
                    var edited = await createModal({
                        title: "Edit encrypted text",
                        value: plain,
                        multiline: true,
                        okText: "Save",
                    });
                    if (edited === null) return;
                    var pw = await getMasterPassword();
                    if (pw === null) return;
                    var nextToken = await encryptToken(edited, pw, addon.iterations);
                    var pos = marker.find();
                    if (!pos) return;
                    cm.replaceRange(nextToken, pos.from, pos.to);
                    showToastSafe("Encrypted text updated");
                } catch (err) {
                    showToastSafe("Unable to edit encrypted text");
                }
            })();
        });

        return marker;
    };

    fold_1.registerFolder("encrypt", exports.EncryptFolder, true);

    exports.defaultOption = {
        enabled: true,
        iterations: 210000,
        mask: "****",
    };
    exports.suggestedOption = {
        enabled: true,
    };
    core_1.suggestedEditorConfig.hmdFoldEncrypt = exports.suggestedOption;

    CodeMirror.defineOption("hmdFoldEncrypt", exports.defaultOption, function (cm, newVal) {
        if (!newVal || typeof newVal === "boolean") {
            newVal = { enabled: !!newVal };
        }
        var inst = exports.getAddon(cm);
        for (var k in exports.defaultOption) {
            inst[k] = (k in newVal) ? newVal[k] : exports.defaultOption[k];
        }
    });

    var FoldEncrypt = /** @class */ (function () {
        function FoldEncrypt(cm) {
            this.cm = cm;
            this.enabled = true;
            this.iterations = 210000;
            this.mask = "****";
        }
        FoldEncrypt.prototype.encryptSelection = async function () {
            var cm = this.cm;
            var selected = cm.getSelection();
            var plain = selected;
            if (!plain) {
                plain = await createModal({
                    title: "Text to encrypt",
                    multiline: true,
                    okText: "Encrypt",
                });
                if (plain === null || plain === "") return;
            }
            var pw = await getMasterPassword();
            if (pw === null) return;
            var token = await encryptToken(plain, pw, this.iterations);
            if (selected) {
                cm.replaceSelection(token, "around");
            } else {
                cm.replaceSelection(token);
            }
            showToastSafe("Text encrypted");
        };
        return FoldEncrypt;
    }());
    exports.FoldEncrypt = FoldEncrypt;

    exports.getAddon = core_1.Addon.Getter("FoldEncrypt", FoldEncrypt, exports.defaultOption);

    exports.encryptSelection = async function (cm) {
        var addon = exports.getAddon(cm);
        return await addon.encryptSelection();
    };

    exports.clearMasterPasswordCache = clearCachedPassword;
    exports.getMasterPasswordCacheMsLeft = function () {
        if (!passwordCache.value) return 0;
        var left = passwordCache.expiresAt - nowMs();
        return left > 0 ? left : 0;
    };
});
