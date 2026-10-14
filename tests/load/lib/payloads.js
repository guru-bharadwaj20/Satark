// Synthetic, PII-free inputs. Phone numbers and UPI IDs use reserved or
// obviously fake values.
export const texts = [
  { text: 'Dear customer your SBI account will be blocked today. Update PAN at http://sbi-kyc.example.test', lang: 'en' },
  { text: 'Electricity bill unpaid. Power will be cut at 9:30 pm. Call 98XXXXXX10 immediately', lang: 'en' },
  { text: 'आपका पार्सल रुका हुआ है, शुल्क ₹25 भरें: http://indiapost-track.example.test', lang: 'hi' },
  { text: 'ನಿಮ್ಮ ಬ್ಯಾಂಕ್ ಖಾತೆ ಸ್ಥಗಿತಗೊಳ್ಳುತ್ತದೆ, ತಕ್ಷಣ OTP ಹಂಚಿಕೊಳ್ಳಿ', lang: 'kn' },
  { text: 'Hi, reaching in 10 minutes. Order the chai.', lang: 'en' },
  { text: 'Work from home job! Earn 5000 daily by liking videos. Pay 500 registration to fake@upi', lang: 'en' },
];

export const urls = [
  'https://example.com/',
  'http://bit.example.test/abc123',
  'https://paytm-refund.example.test/claim',
  'https://www.wikipedia.org/',
];

export function pick(list, i) {
  return list[i % list.length];
}
