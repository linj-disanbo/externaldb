package seqmark

// Mapping seq_marker mapping：seq/height 用 long（要 sort、range），hash/type 用于校验
const Mapping = `{
    "mappings":{
        "properties":{
            "seq":{
                "type":"long"
            },
            "height":{
                "type":"long"
            },
            "hash":{
                "type":"keyword"
            },
            "type":{
                "type":"long"
            }
        }
    }
}`
