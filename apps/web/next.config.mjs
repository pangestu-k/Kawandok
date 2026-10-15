const nextConfig = {
  reactStrictMode: true,
  experimental: {
    optimizePackageImports: ["antd", "@ant-design/icons", "@tanstack/react-query"],
  },
  async rewrites() {
    const target = process.env.API_PROXY_TARGET || "http://127.0.0.1:18080";
    return [
      {
        source: "/api/:path*",
        destination: `${target}/:path*`,
      },
    ];
  },
};

export default nextConfig;
