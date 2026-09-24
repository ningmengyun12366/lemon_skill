/** Content inspection is independent of filenames and directories. */
export function assertArchiveTextPayload(filename, payload) {
  const signatures = [
    [Buffer.from([0, 97, 115, 109]), "WebAssembly"],
    [Buffer.from([0x7f, 69, 76, 70]), "ELF"],
    [Buffer.from("MZ"), "PE"], [Buffer.from("%PDF-"), "PDF"],
    [Buffer.from("GIF87a"), "GIF"], [Buffer.from("GIF89a"), "GIF"],
    [Buffer.from([0x50, 0x4b, 3, 4]), "ZIP"],
  ];
  for (const [signature, kind] of signatures) {
    if (payload.subarray(0, signature.length).equals(signature)) {
      throw new Error("安装 ZIP 不允许二进制文件：" + filename + " (" + kind + ")");
    }
  }
  let text;
  try { text = new TextDecoder("utf-8", { fatal: true }).decode(payload); }
  catch { throw new Error("安装 ZIP 文件不是 UTF-8 文本：" + filename); }
  if (/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(text)) {
    throw new Error("安装 ZIP 文件含二进制控制字符：" + filename);
  }
}
