// guardByName keeps a dangerous button asleep until the name of what it
// acts on is typed exactly into input, so a stray click cannot delete. It
// answers a function that re-arms the guard for another name.
export function guardByName(input, button, name) {
  let wanted = name;
  const check = () => { button.disabled = input.value !== wanted; };
  input.addEventListener("input", check);
  input.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !button.disabled) button.click();
  });
  const arm = (next) => {
    wanted = next;
    input.value = "";
    input.placeholder = next;
    check();
  };
  arm(name);
  return arm;
}
