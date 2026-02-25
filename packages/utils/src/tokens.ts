export function countTokenFast(...inputs: string[]) {
  const data = inputs.join("\n\n");
  let newlineCount = 0;
  for (let idx = 0; idx < data.length; idx += 1) {
    if (data[idx] === "\n") {
      newlineCount += 1;
    }
  }

  return newlineCount + Math.round(data.length / 4);
}
