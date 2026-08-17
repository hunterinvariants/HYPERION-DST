(defproject promtact-jepsen "0.1.0"
  :description "Live linearizability verification for Promtact"
  :dependencies [[org.clojure/clojure "1.11.1"]
                 [jepsen "0.3.9"]]
  :main promtact.core
  :jvm-opts ["-Djava.awt.headless=true"])
