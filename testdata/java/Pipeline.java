public class Pipeline {

    public static void main(String[] args) {
        try {
            String result = runPipeline("input.csv");
            System.out.println("result: " + result);
        } catch (Exception e) {
            System.err.println("error: " + e.getMessage());
        }
    }

    static String runPipeline(String path) throws Exception {
        String raw = loadFile(path);
        String validated = validate(raw);
        return transform(validated);
    }

    static String loadFile(String path) {
        return "data from " + path;
    }

    static String validate(String data) throws Exception {
        if (data == null || data.isEmpty()) {
            throw new Exception("empty data");
        }
        return sanitize(data);
    }

    static String sanitize(String data) {
        return data.trim();
    }

    static String transform(String data) {
        return encode(data);
    }

    static String encode(String s) {
        return "[" + s + "]";
    }
}
