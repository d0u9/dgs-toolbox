/**
 * 设备定位。每个平台用它自己真正能用的方式：
 *
 *   iOS / Android    navigator.geolocation
 *   macOS            CoreLocationCLI --json
 *   Windows / Linux  没有可用方式，直接返回 null
 *
 * 桌面端 Electron 的 navigator.geolocation 走 Google 定位服务、没有 API key，
 * 必然失败，所以桌面端一次都不试它。
 *
 * 拿不到坐标一律返回 null（不抛错），由 getLocation.js 回退到 IP 查询。
 * 拿到了坐标但反查不到地名时，仍然把坐标交出去，地名留空 —— 坐标是准的，
 * 不该跟着地名一起被扔掉。
 */

// 定位本身的超时。宁可早点失败去走 IP，也不要让新建笔记卡在这里。
// 本机 CoreLocationCLI 正常时 0.3 秒返回，6 秒是给唤醒和权限判定留的余量。
const TIMEOUT_MS = 6000;

// 浏览器自己的 timeout 只在开始定位之后才计时，权限弹窗卡住时不会触发，
// 所以外面再套一层硬超时，避免 promise 永远挂着。
const HARD_TIMEOUT_PADDING_MS = 3000;

// 系统手上有这么新的定位点就直接拿来用，不唤醒 GPS 芯片。
// 用户脚本每次执行都会被重新加载，留不住内存缓存，这里靠的是系统那份。
const MAX_AGE_MS = 15 * 60 * 1000;

// 写日记只要城市级精度，关掉高精度走 wifi/基站，通常一秒内出结果。
const HIGH_ACCURACY = false;

// Obsidian 是 GUI 程序，PATH 里通常没有 /opt/homebrew/bin 和 /usr/local/bin，
// 不能指望 execFile 靠 PATH 找得到，得自己按常见位置逐个试。
const CORE_LOCATION_CANDIDATES = [
    "/opt/homebrew/bin/CoreLocationCLI",
    "/usr/local/bin/CoreLocationCLI",
    "/Applications/CoreLocationCLI.app/Contents/MacOS/CoreLocationCLI"
];

// Templater 给用户脚本注入的 require 在移动端会返回 undefined 而不是抛错，
// 但第三方环境不保证，所以还是包一层。
const nodeModule = (name) => {
    try {
        return typeof require === "function" ? require(name) : null;
    } catch (error) {
        console.warn(`require("${name}") failed:`, error);
        return null;
    }
};

const hasBrowserGeolocation = (tp) => Boolean(
    tp.obsidian.Platform?.isMobileApp
    && typeof navigator !== "undefined"
    && navigator.geolocation
);

const isMac = (tp) => Boolean(
    tp.obsidian.Platform?.isDesktopApp && tp.obsidian.Platform?.isMacOS
);

const coreLocationBin = () => {
    const fs = nodeModule("fs");
    if (!fs) return null;

    for (const candidate of CORE_LOCATION_CANDIDATES) {
        try {
            if (fs.existsSync(candidate)) return candidate;
        } catch (error) {
            console.warn(`Checking ${candidate} failed:`, error);
        }
    }

    return null;
};

const getBrowserPosition = () => new Promise((resolve, reject) => {
    let settled = false;

    const settle = (fn, value) => {
        if (settled) return;
        settled = true;
        fn(value);
    };

    const hardTimeout = setTimeout(
        () => settle(reject, new Error("Geolocation never called back")),
        TIMEOUT_MS + HARD_TIMEOUT_PADDING_MS
    );

    navigator.geolocation.getCurrentPosition(
        (position) => {
            clearTimeout(hardTimeout);
            settle(resolve, {
                latitude: position.coords.latitude,
                longitude: position.coords.longitude,
                accuracyMeters: position.coords.accuracy
            });
        },
        (error) => {
            clearTimeout(hardTimeout);
            settle(reject, error);
        },
        {
            enableHighAccuracy: HIGH_ACCURACY,
            timeout: TIMEOUT_MS,
            maximumAge: MAX_AGE_MS
        }
    );
});

// CoreLocationCLI 的数值字段全是字符串，得自己转。
const parseCoreLocation = (stdout) => {
    const line = String(stdout ?? "")
        .split("\n")
        .map((value) => value.trim())
        .find(Boolean);

    if (!line) throw new Error("CoreLocationCLI returned nothing");

    const data = JSON.parse(line);
    const latitude = Number(data.latitude);
    const longitude = Number(data.longitude);

    if (!Number.isFinite(latitude) || !Number.isFinite(longitude)) {
        throw new Error("CoreLocationCLI returned no usable coordinates");
    }

    const accuracy = Number(data.h_accuracy);

    return {
        latitude,
        longitude,
        // h_accuracy 为 -1 表示这个值无效。
        accuracyMeters: Number.isFinite(accuracy) && accuracy >= 0 ? accuracy : null
    };
};

/**
 * CoreLocationCLI --json 打印一行 JSON 后自己退出，不需要 --watch。
 *
 * 它自己也会反查地名，但给的是 locality="Epping"、administrativeArea="NSW"，
 * 和移动端那条链的 "Sydney" / "New South Wales" 对不上。两台设备要写进同一份
 * frontmatter，所以这里只取坐标，地名统一交给 reverseGeocode。
 */
const getCoreLocation = (bin) => new Promise((resolve, reject) => {
    const childProcess = nodeModule("child_process");

    if (!childProcess) {
        reject(new Error("child_process unavailable"));
        return;
    }

    childProcess.execFile(
        bin,
        ["--json"],
        { timeout: TIMEOUT_MS, killSignal: "SIGKILL" },
        (error, stdout, stderr) => {
            // 系统设置里没给定位权限时它会失败，这时回退 IP 是对的。
            if (error) {
                reject(new Error(String(stderr ?? "").trim() || error.message));
                return;
            }

            try {
                resolve(parseCoreLocation(stdout));
            } catch (parseError) {
                reject(parseError);
            }
        }
    );
});

module.exports = async function getDeviceLocation(tp, requestJson) {
    let coordinates = null;

    try {
        if (hasBrowserGeolocation(tp)) {
            coordinates = await getBrowserPosition();
        } else if (isMac(tp)) {
            const bin = coreLocationBin();
            if (!bin) return null;

            coordinates = await getCoreLocation(bin);
        }
    } catch (error) {
        console.warn("Device location lookup failed:", error);
        return null;
    }

    // Windows / Linux 走到这里，本来就没有可用的定位方式。
    if (!coordinates) return null;

    const place = await tp.user.reverseGeocode(
        tp,
        requestJson,
        coordinates.latitude,
        coordinates.longitude
    );

    // place 为 null 时地名全部留空，坐标照给。
    return {
        city: "",
        locality: "",
        region: "",
        country: "",
        countryCode: "",
        ...place,
        ...coordinates
    };
};
