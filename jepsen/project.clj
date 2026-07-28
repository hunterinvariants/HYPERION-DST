(defproject hyperion-jepsen "0.1.0"
  :description "Live linearizability verification for HYPERION-DST"
  :dependencies [[org.clojure/clojure "1.11.1"]
                 [jepsen "0.3.9"]
                 [org.slf4j/slf4j-simple "2.0.13"]]
  :main hyperion.core
  :jvm-opts ["-Djava.awt.headless=true"])
