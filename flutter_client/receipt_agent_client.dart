// Minimal Dart client for the receipt-agent HTTP API.
// Add to pubspec.yaml:  http: ^1.2.0
// NOTE: written to match the Go API in this repo; not yet compiled inside the Doc Scanner app.
import 'dart:convert';
import 'package:http/http.dart' as http;

class Receipt {
  final String id, merchant, date, currency, category;
  final double total;
  Receipt.fromJson(Map<String, dynamic> j)
      : id = j['id'] ?? '',
        merchant = j['merchant'] ?? '',
        date = j['date'] ?? '',
        currency = j['currency'] ?? '',
        category = j['category'] ?? '',
        total = (j['total'] as num).toDouble();
}

class AgentAnswer {
  final String answer;
  final int turns;
  AgentAnswer(this.answer, this.turns);
}

class ReceiptAgentClient {
  final String baseUrl;
  final String? apiKey;
  final http.Client _http;
  ReceiptAgentClient(this.baseUrl, {this.apiKey, http.Client? client})
      : _http = client ?? http.Client();

  Map<String, String> get _headers => {
        'content-type': 'application/json',
        if (apiKey != null) 'x-api-key': apiKey!,
      };

  Future<AgentAnswer> _post(String path, Map<String, String> body) async {
    final r = await _http
        .post(Uri.parse('$baseUrl$path'), headers: _headers, body: jsonEncode(body))
        .timeout(const Duration(seconds: 100));
    if (r.statusCode != 200) {
      throw Exception('receipt-agent ${r.statusCode}: ${r.body}');
    }
    final j = jsonDecode(r.body) as Map<String, dynamic>;
    return AgentAnswer(j['answer'] ?? '', j['turns'] ?? 0);
  }

  /// Send on-device OCR text (e.g. from Google ML Kit) to be cleaned and saved.
  Future<AgentAnswer> ingest(String ocrText) => _post('/v1/receipts', {'ocr_text': ocrText});

  /// Ask a question, e.g. "How much did I spend on food in October?"
  Future<AgentAnswer> ask(String question) => _post('/v1/ask', {'question': question});

  Future<List<Receipt>> list({String? category}) async {
    final uri = Uri.parse('$baseUrl/v1/receipts')
        .replace(queryParameters: {if (category != null) 'category': category});
    final r = await _http.get(uri, headers: _headers);
    if (r.statusCode != 200) throw Exception('receipt-agent ${r.statusCode}');
    return (jsonDecode(r.body) as List)
        .map((e) => Receipt.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}
