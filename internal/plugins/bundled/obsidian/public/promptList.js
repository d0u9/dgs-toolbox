module.exports = async function promptList(tp, label, separator = ",") {
    const input = await tp.system.prompt(`${label}（用逗号分隔）`, "");

    return (input ?? "")
        .split(separator)
        .map((value) => value.trim())
        .filter(Boolean);
};
