/**
 * options 会原样并进 requestUrl 的参数，用来带自定义请求头
 * （Nominatim 要求调用方在 User-Agent 里标明身份）。
 */
module.exports = async function requestJson(tp, url, timeoutMs = 3000, options = {}) {
    let timeoutId;

    const timeout = new Promise((_, reject) => {
        timeoutId = setTimeout(
            () => reject(new Error(`Request timed out after ${timeoutMs} ms`)),
            timeoutMs
        );
    });

    try {
        const response = await Promise.race([
            tp.obsidian.requestUrl({ url, ...options }),
            timeout
        ]);

        return response.json;
    } finally {
        clearTimeout(timeoutId);
    }
};
