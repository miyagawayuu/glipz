declare module "qrcode" {
  const QRCode: {
    toDataURL(
      text: string,
      options?: {
        width?: number;
        margin?: number;
        errorCorrectionLevel?: "L" | "M" | "Q" | "H";
        color?: { dark?: string; light?: string };
      },
    ): Promise<string>;
  };
  export default QRCode;
}

declare module "turndown" {
  export type Rule = {
    filter: (node: Node) => boolean;
    replacement: (content: string, node: Node) => string;
  };

  export default class TurndownService {
    constructor(options?: Record<string, unknown>);
    addRule(key: string, rule: Rule): void;
    turndown(input: string | Node): string;
  }
}
