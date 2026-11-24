using System;

class Pipeline
{
    static void Main(string[] args)
    {
        try
        {
            string result = RunPipeline("input.csv");
            Console.WriteLine("result: " + result);
        }
        catch (Exception e)
        {
            Console.Error.WriteLine("error: " + e.Message);
        }
    }

    static string RunPipeline(string path)
    {
        string raw = LoadFile(path);
        string validated = Validate(raw);
        return Transform(validated);
    }

    static string LoadFile(string path)
    {
        return "data from " + path;
    }

    static string Validate(string data)
    {
        if (string.IsNullOrEmpty(data))
            throw new ArgumentException("empty data");
        return Sanitize(data);
    }

    static string Sanitize(string data)
    {
        return data.Trim();
    }

    static string Transform(string data)
    {
        return Encode(data);
    }

    static string Encode(string s)
    {
        return "[" + s + "]";
    }
}
