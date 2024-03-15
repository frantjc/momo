import cp from "child_process";
// eslint-disable-next-line no-undef
const yarn = process.env.npm_execpath || "yarn";
cp.execSync(yarn, {
    stdio: "inherit"
});
cp.execSync(`${yarn} run dev`, {
    stdio: "inherit"
});
